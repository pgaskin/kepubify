//go:build go1.17
// +build go1.17

package kepub

import "archive/zip"

func zipCopyImpl(z *zip.Writer, f *zip.File) error {
	return z.Copy(f)
}
