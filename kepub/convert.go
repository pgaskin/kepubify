package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"strings"
	"sync"

	"github.com/pgaskin/htmlstream"
	"github.com/pgaskin/htmlstream/polyglot"
	"github.com/pgaskin/kepubify/v5/internal"
)

// Convert converts the EPUB root r into a new EPUB written to w. If r is a
// [zip.Reader], the original zip metadata and unmodified compressed data are
// preserved where possible.
//
// If the EPUB is already a KEPUB (i.e., the body of a content document contains
// an element with the koboSpan class and a kobo.N.M id), [ErrAlreadyKepub] is
// returned before anything is written to w unless [Options.ReprocessKepub] is
// set.
//
// Some files are filtered from the EPUB:
//
//   - [extra] remove calibre_bookmarks.txt
//   - [extra] remove iBooks metadata
//   - [extra] remove macOS metadata
//   - [extra] remove Windows metadata
//
// For the OPF:
//
//   - [mandatory] add the cover-image property to the cover.
//     Kobo only supports the standardized EPUB3-style method of specifying the
//     cover (`manifest>item[properties="cover-image"]`), but most older EPUBs
//     will reference the manifest item with a meta element like
//     `meta[name="cover"][content="{manifest-item-id}"]`. or just set the
//     manifest item ID to `cover` instead of using `properties`.
//
//   - [extra] remove unnecessary Calibre metadata.
//     Removes extraneous metadata elements commonly added by Calibre.
//
// If processing untrusted EPUBs, r should be a [zip.Reader] or other
// in-memory/sandboxed filesystem, rather than an on-disk filesystem since paths
// are not sanitized.
//
// The output is deterministic and should be byte-identical for conversions
// using the same kepubify/dependency/Go versions.
func Convert(ctx context.Context, w io.Writer, r fs.FS, opt Options) error {
	type FileAction int
	const (
		FileActionCopy             = 0
		FileActionIgnore           = 1
		FileActionTransformContent = 2
		FileActionTransformOPF     = 3
	)

	p := opt.progress()

	if tmp, ok := r.(*zip.ReadCloser); ok {
		r = &tmp.Reader
	}

	p(0, true)

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

	if !opt.ReprocessKepub {
		if kepub, err := epubIsKepub(r, cd); err != nil {
			return fmt.Errorf("read source EPUB: %w", err)
		} else if kepub {
			return ErrAlreadyKepub
		}
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
		if transformFileFilter(f.Name) {
			fileAct[i] = FileActionIgnore
		}
	}

	// we'll manually create the mimetype file
	if i, ok := fileIdx["mimetype"]; ok {
		fileAct[i] = FileActionIgnore
	}

	// initialize the output EPUB
	zw := zip.NewWriter(w)

	// the mimetype file must be first
	if err := epubWriteMimetype(zw); err != nil {
		return fmt.Errorf("write mimetype: %w", err)
	}

	var n int
	written := func() {
		n++
		p(float64(n)/float64(len(files)), false)
	}

	// we don't need to do anything with files to be removed

	// copy the unchanged files first
	for i, f := range files {
		if fileAct[i] != FileActionCopy {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		if zr, ok := r.(*zip.Reader); ok {
			err = zw.Copy(zr.File[i])
		} else {
			err = zipCopyFS(zw, f, r)
		}
		if err != nil {
			return fmt.Errorf("copy %q to output EPUB: %w", f.Name, err)
		}
		written()
	}

	// transform the OPF, adding the dummy titlepage if needed
	{
		f := files[fileIdx[opf]]
		rc, err := r.Open(f.Name)
		if err != nil {
			return fmt.Errorf("transform %q: %w", f.Name, err)
		}
		buf := bytes.NewBuffer(nil)
		err = transformOPF(buf, rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("transform %q: %w", f.Name, err)
		}
		if fn, tp, add, err := transformDummyTitlepage(r, opf, buf, opt); err != nil {
			return fmt.Errorf("transform %q: %w", f.Name, err)
		} else if add {
			fh := &zip.FileHeader{
				Name:   fn,
				Method: zip.Deflate,
			}
			fh.SetMode(0666)
			if err := zipReplace(zw, fh, tp); err != nil {
				return fmt.Errorf("write new file %q to output EPUB: %w", fn, err)
			}
		}
		if err := zipReplace(zw, f, buf); err != nil {
			return fmt.Errorf("write %q to output EPUB: %w", f.Name, err)
		}
		written()
	}

	// transform the content documents in parallel, writing them in order
	var content []int
	for i := range files {
		if fileAct[i] == FileActionTransformContent {
			content = append(content, i)
		}
	}

	// reuse a pool of buffers
	type transformed struct {
		index int
		buf   *bytes.Buffer
	}
	pool := &sync.Pool{
		New: func() any {
			return bytes.NewBuffer(nil)
		},
	}
	for t := range internal.Parallel(ctx, opt.workers(), content, func(i int) (transformed, error) {
		f := files[i]
		rc, err := r.Open(f.Name)
		if err != nil {
			return transformed{}, fmt.Errorf("transform %q: %w", f.Name, err)
		}
		buf := pool.Get().(*bytes.Buffer)
		err = transformContent(buf, rc, opt, f.Name)
		rc.Close()
		if err != nil {
			buf.Reset()
			pool.Put(buf)
			return transformed{}, fmt.Errorf("transform %q: %w", f.Name, err)
		}
		return transformed{i, buf}, nil
	})(&err) {
		f := files[t.index]
		if err := zipReplace(zw, f, t.buf); err != nil {
			return fmt.Errorf("write %q to output EPUB: %w", f.Name, err)
		}
		t.buf.Reset()
		pool.Put(t.buf)
		written()
	}
	if err != nil {
		return err
	}

	// finalize the output
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize output EPUB: %w", err)
	}

	p(1, true)
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

// epubIsKepub reports whether any of the content documents cd contain a Kobo
// span (see hasKoboSpan). Content documents which can't be opened are ignored
// like in Convert.
func epubIsKepub(epub fs.FS, cd []string) (bool, error) {
	for _, fn := range cd {
		f, err := epub.Open(fn)
		if err != nil {
			continue
		}

		// cheap check first so most EPUBs don't need to be parsed
		found, err := readerContains(f, []byte("koboSpan"))
		f.Close()
		if err != nil {
			return false, fmt.Errorf("read %q: %w", fn, err)
		}
		if !found {
			continue
		}

		// the string could be in a comment, style, script, or text
		f, err = epub.Open(fn)
		if err != nil {
			continue
		}
		found, err = hasKoboSpan(f)
		f.Close()
		if err != nil {
			return false, fmt.Errorf("parse %q: %w", fn, err)
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func hasKoboSpan(r io.Reader) (bool, error) {
	var errFound = errors.New("found") // sentinel error so we can stop reading

	var body bool
	err := convert(io.Discard, r, polyglot.Options{
		OnTag: func(e *polyglot.Tag) {
			switch {
			case e.NS == htmlstream.NamespaceXHTML && e.Local == "body":
				body = e.Start
			case body && e.Start && e.Token != nil && isKoboSpan(e.Token) && isKoboSpanID(attrValue(e.Token, "id")):
				e.Writer().Fail(errFound)
			}
		},
	})
	if errors.Is(err, errFound) {
		return true, nil
	}
	return false, err
}

func isKoboSpanID(id string) bool {
	rest, ok := strings.CutPrefix(id, "kobo.")
	if !ok {
		return false
	}
	para, seg, ok := strings.Cut(rest, ".")
	return ok && isDigits(para) && isDigits(seg)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) != 0
}

func readerContains(r io.Reader, needle []byte) (bool, error) {
	buf := make([]byte, 32*1024)
	keep := 0 // bytes kept from the previous chunk so a match across chunks is found
	for {
		n, err := r.Read(buf[keep:])
		end := keep + n
		if bytes.Contains(buf[:end], needle) {
			return true, nil
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		keep = min(len(needle)-1, end)
		copy(buf, buf[end-keep:end])
	}
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
		Extra:         zipExtra(f),
		ExternalAttrs: f.ExternalAttrs,
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

// zipExtra returns the extra fields of f for a new header written with
// [zip.Writer.CreateHeader], leaving out malformed fields (#97), the zip64
// field, and if f.Modified is set, the extended timestamp field (which will be
// added by [zip.Writer.CreateHeader]).
func zipExtra(f *zip.FileHeader) []byte {
	const (
		zip64ExtraID   = 0x0001
		extTimeExtraID = 0x5455
	)
	var extra []byte
	for b := f.Extra; len(b) >= 4; {
		id, size := binary.LittleEndian.Uint16(b), int(binary.LittleEndian.Uint16(b[2:]))
		if 4+size > len(b) {
			break
		}
		if id != zip64ExtraID && (id != extTimeExtraID || f.Modified.IsZero()) {
			extra = append(extra, b[:4+size]...)
		}
		b = b[4+size:]
	}
	return extra
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
		Extra:         zipExtra(f),
		ExternalAttrs: f.ExternalAttrs,
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(w, rc)
	return err
}

// transformFileFilter returns true if a file should be filtered from the EPUB.
func transformFileFilter(fn string) bool {
	switch path.Base(fn) {
	case "calibre_bookmarks.txt": // Calibre
		return true
	case "iTunesMetadata.plist", "iTunesArtwork.plist": // iBooks
		return true
	case ".DS_Store", ".DS_STORE": // macOS
		return true
	case "thumbs.db", "Thumbs.db": // Windows
		return true
	}
	return strings.HasPrefix(fn, "__MACOSX/") // macOS archivers
}
