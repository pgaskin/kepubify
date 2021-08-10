//go:build go1.17 || zip117
// +build go1.17 zip117

package kepub

import "github.com/pgaskin/kepubify/v4/internal/zip"

func zipCopyImpl(z *zip.Writer, f *zip.File) error {
	return z.Copy(f)
}
