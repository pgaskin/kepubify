package kobo

import (
	"errors"
	"os"
	"path/filepath"
)

var findFuncs = []func() ([]string, error){}

// ErrCommandNotFound is thrown when a required command is not found.
var ErrCommandNotFound = errors.New("required command not found")

// Find gets the paths to the kobos.
func Find() ([]string, error) {
	kobos := []string{}
	seen := map[string]bool{}
	for _, fn := range findFuncs {
		k, err := fn()
		if err != nil {
			if err == ErrCommandNotFound {
				continue
			}
			return nil, err
		}
		for _, kobo := range k {
			if !seen[kobo] {
				kobos = append(kobos, kobo)
				seen[kobo] = true
			}
		}
	}
	return kobos, nil
}

// IsKobo checks if a path is a kobo.
func IsKobo(path string) bool {
	if fi, err := os.Stat(filepath.Join(path, ".kobo")); err != nil || !fi.IsDir() {
		return false
	}
	return true
}
