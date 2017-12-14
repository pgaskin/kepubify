package kepub

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// exists checks whether a path exists
func exists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return true
	}
	return false
}

// isDir checks if a exists and is a dir
func isDir(path string) bool {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return true
	}
	return false
}

// UnpackEPUB unpacks an epub
func UnpackEPUB(src, dest string, overwritedest bool) error {
	if src == "" {
		return errors.New("source must not be empty")
	}

	if dest == "" {
		return errors.New("destination must not be empty")
	}

	if !exists(src) {
		return fmt.Errorf(`source file "%s" does not exist`, src)
	}

	if exists(dest) {
		if !overwritedest {
			return fmt.Errorf(`destination "%s" already exists`, dest)
		}
		os.RemoveAll(dest)
	}

	src, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("error resolving absolute path of source: %s", err)
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("error resolving absolute path of destination: %s", err)
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0700)
		} else {
			os.MkdirAll(filepath.Dir(path), 0700)
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

// PackEPUB packs an epub
func PackEPUB(src, dest string, overwritedest bool) error {
	if src == "" {
		return errors.New("source dir must not be empty")
	}

	if dest == "" {
		return errors.New("destination must not be empty")
	}

	if !exists(src) {
		return fmt.Errorf(`source dir "%s" does not exist`, src)
	}

	if exists(dest) {
		if !overwritedest {
			return fmt.Errorf(`destination "%s" already exists`, dest)
		}
		os.RemoveAll(dest)
	}

	src, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("error resolving absolute path of sourcedir: %s", err)
	}

	dest, err = filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("error resolving absolute path of destination: %s", err)
	}

	if !exists(filepath.Join(src, "META-INF", "container.xml")) {
		return fmt.Errorf("could not find META-INF/container.xml")
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("error creating destination file: %s", err)
	}
	defer f.Close()

	epub := zip.NewWriter(f)
	defer epub.Close()

	var addFile = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the path of the file relative to the source folder
		relativePath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf(`error getting relative path of "%s"`, path)
		}

		// Fix issue with path separators in zip on windows
		relativePath = filepath.ToSlash(relativePath)

		if filepath.ToSlash(path) == filepath.ToSlash(dest) {
			// Skip if it is trying to pack itself
			return nil
		}

		if !info.Mode().IsRegular() {
			// Skip if not file
			return nil
		}

		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(src, "mimetype")) {
			// Skip if it is the mimetype file
			return nil
		}

		// Create file in zip
		w, err := epub.Create(relativePath)
		if err != nil {
			return fmt.Errorf(`error creating file in epub "%s": %s`, relativePath, err)
		}

		// Open file on disk
		r, err := os.Open(path)
		if err != nil {
			return fmt.Errorf(`error reading file "%s": %s`, path, err)
		}
		defer r.Close()

		// Write file from disk to epub
		_, err = io.Copy(w, r)
		if err != nil {
			return fmt.Errorf(`error writing file to epub "%s": %s`, relativePath, err)
		}

		return nil
	}

	// Do not compress mimetype
	mimetypeWriter, err := epub.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})

	if err != nil {
		return errors.New("error writing mimetype to epub")
	}

	_, err = mimetypeWriter.Write([]byte("application/epub+zip"))
	if err != nil {
		return errors.New("error writing mimetype to epub")
	}

	err = filepath.Walk(src, addFile)
	if err != nil {
		return fmt.Errorf("error adding file to epub: %s", err)
	}

	return nil
}
