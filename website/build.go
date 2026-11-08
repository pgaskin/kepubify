package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// note: this is loosely based on what I did for github.com/pgaskin/htmlstream@v0.0.4/web/polyglottify

const (
	outDir   = "dist"
	tryDir   = "dist/try"
	kepubify = "node_modules/kepubify" // the npm package
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	if _, err := os.Stat(filepath.FromSlash("src/index.ts")); err != nil {
		fatal("this script must be run from the website directory", err)
	}
	if _, err := os.Stat("node_modules"); err != nil {
		fatal("npm packages not installed, run npm ci", err)
	}
	if _, err := os.Stat(filepath.FromSlash(kepubify + "/dist/kepubify.wasm")); err != nil {
		fatal("kepubify package not built, run npm run build in ../js", err)
	}
	if err := build(); err != nil {
		fatal("build", err)
	}
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func run(cmd *exec.Cmd) error {
	slog.Info("run", "cmd", cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	start := time.Now()
	err := cmd.Run()
	if err != nil {
		slog.Error("run failed", "cmd", cmd.Path, "err", err)
		return err
	}
	slog.Info("run done", "cmd", cmd.Path, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func build() error {
	if err := run(exec.Command(filepath.FromSlash("node_modules/.bin/tsc"), "-p", "tsconfig.json")); err != nil {
		return fmt.Errorf("tsc: %w", err)
	}

	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.CopyFS(outDir, os.DirFS("static")); err != nil {
		return fmt.Errorf("copy static: %w", err)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	if b, err := os.ReadFile(filepath.FromSlash(kepubify + "/package.json")); err != nil {
		return err
	} else if err := json.Unmarshal(b, &pkg); err != nil {
		return fmt.Errorf("parse kepubify package.json: %w", err)
	}

	// TODO: version from git since we deploy the website on every commit?
	js, err := bundle(tryDir, map[string]string{"KEPUBIFY_VERSION": pkg.Version},
		"src/index.ts", kepubify+"/dist/kepubify.worker.js")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tryDir, 0o755); err != nil {
		return err
	}
	for name, data := range js {
		if err := os.WriteFile(filepath.Join(tryDir, name), data, 0o644); err != nil {
			return err
		}
	}

	wasm, err := os.ReadFile(filepath.FromSlash(kepubify + "/dist/kepubify.wasm"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tryDir, "kepubify.wasm"), wasm, 0o644); err != nil {
		return err
	}

	var static int
	if err := fs.WalkDir(os.DirFS("static"), ".", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			static++
		}
		return err
	}); err != nil {
		return err
	}
	fmt.Printf("%-32s %8d files\n", "static", static)
	for _, name := range slices.Sorted(maps.Keys(js)) {
		fmt.Printf("%-32s %8d bytes\n", "try/"+name, len(js[name]))
	}
	fmt.Printf("%-32s %8d bytes (kepubify %s)\n", "try/kepubify.wasm", len(wasm), pkg.Version)
	return nil
}

func bundle(outDir string, define map[string]string, entries ...string) (map[string][]byte, error) {
	slog.Info("esbuild", "op", "bundle", "entries", entries)
	defines := map[string]string{}
	for k, v := range define {
		b, _ := json.Marshal(v)
		defines[k] = string(b)
	}
	res := api.Build(api.BuildOptions{
		EntryPoints:       entries,
		Outdir:            outDir,
		EntryNames:        "[name]",
		Bundle:            true,
		Splitting:         true,
		Format:            api.FormatESModule,
		Target:            api.ES2022,
		Charset:           api.CharsetUTF8,
		Platform:          api.PlatformBrowser,
		External:          []string{"node:*"}, // kepubify's node fallback for loading the module
		Define:            defines,
		Sourcemap:         api.SourceMapLinked,
		TreeShaking:       api.TreeShakingTrue,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsNone,
		LogLevel:          api.LogLevelSilent,
		Write:             false,
	})
	if len(res.Errors) != 0 {
		return nil, fmt.Errorf("esbuild: %s", strings.TrimSpace(strings.Join(api.FormatMessages(res.Errors, api.FormatMessagesOptions{}), "")))
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, f := range res.OutputFiles {
		name, err := filepath.Rel(abs, f.Path)
		if err != nil || name != filepath.Base(name) {
			return nil, fmt.Errorf("unexpected output file %q", f.Path)
		}
		files[name] = f.Contents
	}
	if _, ok := files["kepubify.worker.js"]; !ok {
		return nil, errors.New("no kepubify.worker.js in the output")
	}
	return files, nil
}

// TODO: process CSS?
// TODO: html tweaks?
