package main

import (
	"io/ioutil"
	"os"
	"strings"
)

// isDir checks if a exists and is a dir
func isDir(path string) bool {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return true
	}
	return false
}

// isFile checks if a exists and is a file
func isFile(path string) bool {
	if exists(path) && !isDir(path) {
		return true
	}
	return false
}

// exists checks whether a path exists
func exists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return true
	}
	return false
}

// exists checks whether a path exists
func isEmptyDir(path string) bool {
	if !isDir(path) {
		return false
	}

	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return false
	}

	for _, info := range infos {
		if strings.HasPrefix(info.Name(), ".") || info.Name() == "thumbs.db" || info.Name() == ".DS_STORE" {
			continue
		}

		return false
	}

	return true
}
