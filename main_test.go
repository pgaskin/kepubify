package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCopyAttrs(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	for _, name := range []string{src, dst} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, time.Time{}, mtime); err != nil {
		t.Fatal(err)
	}
	if err := copyAttrs(src, dst); err != nil {
		t.Fatalf("preserve: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("modification time %v, want %v", fi.ModTime(), mtime)
	}
	if want, _ := os.Stat(src); fi.Mode().Perm() != want.Mode().Perm() {
		t.Errorf("mode %v, want %v", fi.Mode().Perm(), want.Mode().Perm())
	}
}
