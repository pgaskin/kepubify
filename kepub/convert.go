package kepub

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/pgaskin/kepubify/v4/internal/zip"
	"golang.org/x/sync/errgroup"
)

// Convert converts the EPUB root r into a new EPUB written to w. If r is a
// (*zip.Reader) (from archive/zip by default, or from
// github.com/pgaskin/kepubify/_/go116-zip.go117/archive/zip if the zip117 build
// tag is used (even on Go 1.17)), the original zip metadata is preserved where
// possible, and additional optimizations are applied to prevent re-compressing
// unchanged data where possible. If processing untrusted EPUBs, r should not
// point to an unrestricted on-disk filesystem since paths are not sanitized; it
// should point to a (*zip.Reader) or other in-memory or synthetic filesystem.
func (c *Converter) Convert(ctx context.Context, w io.Writer, r fs.FS) error {
	type FileAction int
	const (
		FileActionCopy             = 0
		FileActionIgnore           = 1
		FileActionTransformContent = 2
		FileActionTransformOPF     = 3
	)

	p := ctxProgress(ctx)

	if tmp, ok := r.(*zip.ReadCloser); ok {
		r = &tmp.Reader
	}

	if p != nil {
		p(true, 0, 0)
	}

	var files []*zip.FileHeader
	if zr, ok := r.(*zip.Reader); ok {
		// if it's a zip, preserve the original FileHeaders
		files = make([]*zip.FileHeader, len(zr.File))
		for i, f := range zr.File {
			files[i] = &f.FileHeader
		}
	} else if err := fs.WalkDir(r, ".", func(path string, d fs.DirEntry, err error) error {
		// if it's not a zip, create fake FileHeaders
		if !d.IsDir() {
			i, err := d.Info()
			if err != nil {
				return fmt.Errorf("get info for %q: %w", path, err)
			}

			// based on zip.FileInfoHeader, but we don't set the mode
			fh := &zip.FileHeader{
				Name:               path,
				Method:             zip.Deflate,
				UncompressedSize64: uint64(i.Size()),
			}
			fh.SetModTime(i.ModTime())
			if fh.UncompressedSize64 > math.MaxUint32 {
				fh.UncompressedSize = math.MaxUint32
			} else {
				fh.UncompressedSize = uint32(fh.UncompressedSize64)
			}

			// because it's an epub (but this won't matter because we skip it)
			if path == "mimetype" {
				fh.Method = zip.Store
			}

			files = append(files, fh)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("find files: %w", err)
	}

	fileAct := make([]FileAction, len(files))
	fileIdx := make(map[string]int, len(files))

	for i, f := range files {
		fileIdx[f.Name] = i
	}

	opf, err := epubPackage(r)
	if err != nil {
		return fmt.Errorf("read source EPUB: %w", err)
	}

	cd, err := epubContentDocuments(r, opf)
	if err != nil {
		return fmt.Errorf("read source EPUB: %w", err)
	}

	// mark the opf to be transformed
	fileAct[fileIdx[opf]] = FileActionTransformOPF

	// mark the content files to be transformed
	for _, fn := range cd {
		if i, ok := fileIdx[fn]; ok {
			fileAct[i] = FileActionTransformContent
		} else {
			// OCF zips are generally case-sensitive, but we'll attempt to do a
			// case-insensitive match if we can't find the file (but that we
			// won't fix the filename case mismatch).
			fn = strings.ToLower(fn)
			for i, f := range files {
				if strings.ToLower(f.Name) == fn {
					fileAct[i] = FileActionTransformContent
					break
				}
			}
			// ignore any failures
		}
	}

	// get rid of any directory entries if they made it here (most likely from a zip which has them)
	for i, f := range files {
		if f.Mode().IsDir() || f.Name[len(f.Name)-1] == '/' {
			fileAct[i] = FileActionIgnore
		}
	}

	// mark the files to be removed
	for i, f := range files {
		if c.TransformFileFilter(f.Name) {
			fileAct[i] = FileActionIgnore
		}
	}

	// we'll manually create the mimetype file
	if i, ok := fileIdx["mimetype"]; ok {
		fileAct[i] = FileActionIgnore
	}

	// start transforming and writing the content files in parallel
	type File struct {
		Index  int             // -1 for a new file
		Header *zip.FileHeader // if Index is -1
		// We could have passed around a *html.Node or a *etree.Document, and
		// encoded it directly to the zip writer, but this gives better
		// performance for a few reasons. Firstly, writing to the zip file can
		// only be done on a single thread, which is also where the compression
		// is being done. By pre-rendering the document, we do as much work as
		// possible in parallel. Secondly, passing around a node results in
		// passing around a complex tree of pointers. By passing just the
		// rendered bytes (which also happens to be more compact in-memory), we
		// reduce the load on the GC and reduce the memory required by files
		// waiting to be written. Thirdly, the impact of passing around bytes is
		// significantly reduced by using a buffer pool rather than passing
		// around new slices each time.
		Bytes *bytes.Buffer
	}

	g, ctx := errgroup.WithContext(ctx)

	pool := &sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(nil)
		},
	}
	queue := make(chan int)
	output := make(chan File)

	g.Go(func() error {
		defer close(queue)

		// we don't need to do anything with files to be removed

		// add copied files to the EPUB first (the order will be preserved)
		for i := range files {
			if fileAct[i] == FileActionCopy {
				select {
				case output <- File{Index: i}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		// then queue the files to be transformed in parallel
		for i := range files {
			if fileAct[i] == FileActionTransformOPF || fileAct[i] == FileActionTransformContent {
				select {
				case queue <- i:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		return nil
	})

	// start the transformation goroutines
	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			for i := range queue {
				f := files[i]

				rc, err := r.Open(f.Name)
				if err != nil {
					return fmt.Errorf("transform %q: %w", f.Name, err)
				}

				buf := pool.Get().(*bytes.Buffer)

				switch a := fileAct[i]; a {
				case FileActionTransformOPF:
					err = c.TransformOPF(buf, rc)
					if err == nil {
						if fn, r, a, err1 := c.TransformDummyTitlepage(r, opf, buf); err1 != nil {
							err = err1
						} else if a {
							buf1 := pool.Get().(*bytes.Buffer)
							if _, err := buf1.ReadFrom(r); err != nil {
								err = fmt.Errorf("apply title page fix: %w", err)
							} else {
								fh := &zip.FileHeader{
									Name:   fn,
									Method: zip.Deflate,
								}
								fh.SetMode(0666)
								output <- File{
									Index:  -1,
									Header: fh,
									Bytes:  buf1,
								}
							}
						}
					}
				case FileActionTransformContent:
					err = c.TransformContent(buf, rc)
				default:
					panic(fmt.Sprintf("unexpected action %d in transformation goroutine", a))
				}

				rc.Close()

				if err != nil {
					buf.Reset()
					pool.Put(buf)
					return fmt.Errorf("transform %q: %w", f.Name, err)
				}

				select {
				case output <- File{Index: i, Bytes: buf}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	go func() {
		// close the channel after we finish to prevent deadlocks on error
		g.Wait()
		close(output)
	}()

	// initialize the output EPUB
	zw := zip.NewWriter(w)

	// the mimetype file must be first
	if err := epubWriteMimetype(zw); err != nil {
		return fmt.Errorf("write mimetype: %w", err)
	}

	// write the files
	var n int
	for of := range output {
		if of.Index == -1 {
			if err := zipReplace(zw, of.Header, of.Bytes); err != nil {
				return fmt.Errorf("write new file %q to output EPUB: %w", of.Header.Name, err)
			}
			of.Bytes.Reset()
			pool.Put(of.Bytes)
			continue
		}
		f := files[of.Index]
		switch b := of.Bytes; b {
		case nil:
			var err error
			if zr, ok := r.(*zip.Reader); ok {
				err = zipCopy(zw, zr.File[of.Index])
			} else {
				err = zipCopyFS(zw, f, r)
			}
			if err != nil {
				return fmt.Errorf("copy %q to output EPUB: %w", f.Name, err)
			}
		default:
			if err := zipReplace(zw, f, b); err != nil {
				return fmt.Errorf("write %q to output EPUB: %w", f.Name, err)
			}
			b.Reset()
			pool.Put(b)
		}
		if p != nil {
			n++
			p(false, n, len(files))
		}
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// finalize the output
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize output EPUB: %w", err)
	}

	if p != nil {
		p(true, n, n)
	}
	return nil
}

// epubWriteMimetype writes the mimetype file to an EPUB. It must be called
// before any other files are written.
func epubWriteMimetype(epub *zip.Writer) error {
	w, err := epub.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "application/epub+zip")
	return err
}

// epubPackage gets the filename of the first OPF package document from the OCF
// container.
//
// OCF containers can have technically more than one package document, but
// like most reading systems, we'll just use the first one.
func epubPackage(epub fs.FS) (string, error) {
	var ocf struct {
		XMLName  xml.Name `xml:"urn:oasis:names:tc:opendocument:xmlns:container container"`
		Version  string   `xml:"version,attr"`
		RootFile []struct {
			FullPath  string `xml:"full-path,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"urn:oasis:names:tc:opendocument:xmlns:container rootfiles>rootfile"`
	}

	// we will be lenient about the mimetype file by not checking for it

	f, err := epub.Open("META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("parse OCF container: %w", err)
	}
	defer f.Close()

	if err := xml.NewDecoder(f).Decode(&ocf); err != nil {
		f.Close()
		return "", fmt.Errorf("parse OCF container: %w", err)
	}

	if ocf.Version != "1.0" {
		return "", fmt.Errorf("parse OCF container: invalid OCF version %q", ocf.Version)
	}

	for _, f := range ocf.RootFile {
		if f.MediaType == "application/oebps-package+xml" {
			return f.FullPath, nil
		}
	}

	return "", fmt.Errorf("parse OCF container: no valid package documents found")
}

// epubContentDocuments gets the XHTML content document filenames in the
// provided EPUB OPF package document.
//
// While application/xhtml+xml is the only officially accepted type (as of EPUB
// 2.0.1 - 3.3), some invalid EPUBs use text/html, and others use
// application/xml or text/xml with a .htm, .xhtml, or .html extension.
func epubContentDocuments(epub fs.FS, pkg string) ([]string, error) {
	var opf struct {
		XMLName      xml.Name `xml:"http://www.idpf.org/2007/opf package"`
		ManifestItem []struct {
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"http://www.idpf.org/2007/opf manifest>item"`
	}

	f, err := epub.Open(pkg)
	if err != nil {
		return nil, fmt.Errorf("parse OPF package: %w", err)
	}
	defer f.Close()

	if err := xml.NewDecoder(f).Decode(&opf); err != nil {
		f.Close()
		return nil, fmt.Errorf("parse OPF package: %w", err)
	}

	var docs []string
	for _, it := range opf.ManifestItem {
		switch it.MediaType {
		case "application/xhtml+xml", "text/html":
			docs = append(docs, path.Join(path.Dir(pkg), it.Href))
			continue
		}
		switch strings.ToLower(path.Ext(it.Href)) {
		case ".htm", ".html", ".xhtml":
			docs = append(docs, path.Join(path.Dir(pkg), it.Href))
			continue
		}
	}

	return docs, nil
}

// zipReplace copies a file from one zip archive to another, preserving the
// metadata, replacing the content, and force-enabling compression.
func zipReplace(z *zip.Writer, f *zip.FileHeader, r io.Reader) error {
	w, err := z.CreateHeader(&zip.FileHeader{
		Name:          f.Name,
		Comment:       f.Comment,
		Method:        zip.Deflate,
		Modified:      f.Modified,
		ModifiedTime:  f.ModifiedTime,
		ModifiedDate:  f.ModifiedDate,
		Extra:         f.Extra,
		ExternalAttrs: f.ExternalAttrs,
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

// zipCopy copies a file from one zip archive to another. On Go 1.17+, this uses
// (*zip.Writer).Copy, which is much faster than reading and re-compressing the
// data.
func zipCopy(z *zip.Writer, f *zip.File) error {
	return zipCopyImpl(z, f)
}

// zipCopy copies a file from a FS to a zip using the information in the
// provided FileHeader.
func zipCopyFS(z *zip.Writer, f *zip.FileHeader, fs fs.FS) error {
	rc, err := fs.Open(f.Name)
	if err != nil {
		return err
	}
	defer rc.Close()

	w, err := z.CreateHeader(&zip.FileHeader{
		Name:          f.Name,
		Comment:       f.Comment,
		Method:        f.Method,
		Modified:      f.Modified,
		ModifiedTime:  f.ModifiedTime,
		ModifiedDate:  f.ModifiedDate,
		Extra:         f.Extra,
		ExternalAttrs: f.ExternalAttrs,
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(w, rc)
	return err
}
