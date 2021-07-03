//go:build generate
// +build generate

package main

import (
	"encoding/base64"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:generate go run -tags generate wasm_exec.go

func main() {
	var r io.Reader
	var c io.Closer
	var err error

	if f, err := os.Open(filepath.Join(build.Default.GOROOT, "misc", "wasm", "​wasm_exec.js")); err == nil {
		r, c = f, f
	} else {
		fmt.Fprintf(os.Stderr, "warning: %s: failed to open wasm_exec.js, downloading it directly: %v\n", os.Args[0], err)

		var url string
		if x := strings.Split(runtime.Version(), "-"); len(x) == 1 {
			url = "https://go.googlesource.com/go/+/refs/tags/" + x[0] + "/misc/wasm/wasm_exec.js?format=TEXT"
		} else {
			url = "https://go.googlesource.com/go/+/" + x[1] + "/misc/wasm/wasm_exec.js?format=TEXT"
		}
		if resp, err := http.Get(url); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: failed to download wasm_exec.js: get %q: %v\n", os.Args[0], url, err)
			os.Exit(1)
			return
		} else if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "error: %s: failed to download wasm_exec.js: get %q: http status %s\n", os.Args[0], url, resp.Status)
			os.Exit(1)
			return
		} else {
			r, c = base64.NewDecoder(base64.StdEncoding, resp.Body), resp.Body
		}
	}

	buf, err := ioutil.ReadAll(r)
	c.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s: failed to read wasm_exec.js: %v\n", os.Args[0], err)
		os.Exit(1)
		return
	}

	if err := os.WriteFile("wasm_exec.js", buf, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s: failed to save wasm_exec.js: %v\n", os.Args[0], err)
		os.Exit(1)
		return
	}
}
