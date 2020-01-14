package kepub

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/beevik/etree"
)

// FindOPF finds the path to the first OPF package document in an unpacked EPUB.
func FindOPF(dir string) (string, error) {
	rsk, err := os.Open(filepath.Join(dir, "META-INF", "container.xml"))
	if err != nil {
		return "", fmt.Errorf("error opening container.xml: %w", err)
	}
	defer rsk.Close()

	doc := etree.NewDocument()
	if _, err = doc.ReadFrom(rsk); err != nil {
		return "", fmt.Errorf("error parsing container.xml: %w", err)
	}

	if e := doc.FindElement("//rootfiles/rootfile[@full-path]"); e != nil {
		if p := e.SelectAttrValue("full-path", ""); p != "" {
			return filepath.Join(dir, p), nil
		}
	}
	return "", errors.New("error parsing container.xml: could not find rootfile")
}

// UnpackEPUB unpacks an EPUB to a directory, which must be a nonexistent
// directory under an existing parent.
func UnpackEPUB(epub, dir string) error {
	if len(epub) == 0 || len(dir) == 0 {
		return fmt.Errorf("epub (%#v) and dir (%#v) must not be empty", epub, dir)
	}

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("destination dir (%#v) must not exist", dir)
	}

	for _, d := range []*string{&epub, &dir} {
		abs, err := filepath.Abs(*d)
		if err != nil {
			return fmt.Errorf("resolve absolute path of %#v: %w", *d, err)
		}
		*d = abs
	}

	zr, err := zip.OpenReader(epub)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err := os.Mkdir(dir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	for _, zf := range zr.File {
		// closure to simplify closing files with defer and prevent leaking FDs
		if err := func(zf *zip.File) error {
			fr, err := zf.Open()
			if err != nil {
				return fmt.Errorf("open zip file %#v: %w", zf, err)
			}
			defer fr.Close()

			out := filepath.Join(dir, zf.Name) // note: this is not safe to use with untrusted EPUBs

			if zf.FileInfo().IsDir() {
				if err := os.MkdirAll(out, 0755); err != nil {
					return fmt.Errorf("extract dir %#v to %#v: %w", zf.Name, out, err)
				}
				return nil
			}

			// some badly-formed zips don't have dirs first
			if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				return fmt.Errorf("create parent dir for %#v: %w", zf.Name, err)
			}

			f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("extract file %#v to %#v: %w", zf.Name, out, err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			if _, err := io.Copy(f, fr); err != nil {
				return fmt.Errorf("extract file %#v to %#v: %w", zf.Name, out, err)
			}

			return nil
		}(zf); err != nil {
			return err
		}
	}

	return nil
}

// PackEPUB unpacks an EPUB to a file.
func PackEPUB(dir, epub string) error {
	if len(epub) == 0 || len(dir) == 0 {
		return fmt.Errorf("epub (%#v) and dir (%#v) must not be empty", epub, dir)
	}

	for _, d := range []*string{&dir, &epub} {
		abs, err := filepath.Abs(*d)
		if err != nil {
			return fmt.Errorf("resolve absolute path of %#v: %w", *d, err)
		}
		*d = abs
	}

	if _, err := os.Stat(filepath.Join(dir, "META-INF", "container.xml")); err != nil {
		return fmt.Errorf("could not access META-INF/container.xml: %w", err)
	}

	f, err := os.OpenFile(epub, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create destination epub: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if err := zw.Close(); err != nil {
			panic(err)
		}
	}()

	if w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}); err != nil {
		return fmt.Errorf("error writing mimetype to epub: %w", err)
	} else if _, err = w.Write([]byte("application/epub+zip")); err != nil {
		return fmt.Errorf("error writing mimetype to epub: %w", err)
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf(`error getting relative path of "%s"`, path)
		}

		if filepath.ToSlash(path) == filepath.ToSlash(epub) {
			return nil // don't pack self
		}

		if !info.Mode().IsRegular() {
			return nil // only pack files
		}

		if filepath.Base(rel) == "mimetype" {
			return nil // don't pack the mimetype again
		}

		rel = filepath.ToSlash(rel) // zips must use a forward slash

		w, err := zw.Create(rel)
		if err != nil {
			return fmt.Errorf(`error creating file in epub %#v: %w`, rel, err)
		}

		r, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return fmt.Errorf(`error reading file %#v: %w`, path, err)
		}
		defer r.Close()

		if _, err = io.Copy(w, r); err != nil {
			return fmt.Errorf(`error writing file to epub %#v: %w`, rel, err)
		}

		return nil
	})
}
