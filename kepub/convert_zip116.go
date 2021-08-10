//go:build go1.16 && !go1.17 && !zip117
// +build go1.16,!go1.17,!zip117

package kepub

import (
	"io"

	"github.com/pgaskin/kepubify/v4/internal/zip"
)

func zipCopyImpl(z *zip.Writer, f *zip.File) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()

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

	_, err = io.Copy(w, r)
	return err
}
