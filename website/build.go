package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/pgaskin/htmlstream"
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

	wasm, err := os.ReadFile(filepath.FromSlash(kepubify + "/dist/kepubify.wasm"))
	if err != nil {
		return err
	}
	worker, err := bundleWorker(kepubify + "/dist/kepubify.worker.js")
	if err != nil {
		return err
	}

	// TODO: version from git since we deploy the website on every commit?
	js, err := bundle(tryDir, map[string]string{"KEPUBIFY_VERSION": pkg.Version}, map[string]inline{
		"kepubify.wasm":   {wasm, api.LoaderBinary},
		"kepubify.worker": {worker, api.LoaderText},
	}, "src/index.ts")
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

	hashed, err := hashAssets(outDir)
	if err != nil {
		return fmt.Errorf("hash assets: %w", err)
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
	fmt.Printf("%-32s %8d urls\n", "hashed", hashed)
	for _, name := range slices.Sorted(maps.Keys(js)) {
		fmt.Printf("%-32s %8d bytes\n", "try/"+name, len(js[name]))
	}
	fmt.Printf("%-32s %8d bytes (kepubify %s, inlined)\n", "kepubify.wasm", len(wasm), pkg.Version)
	fmt.Printf("%-32s %8d bytes (inlined)\n", "kepubify.worker.js", len(worker))
	return nil
}

type inline struct {
	data   []byte
	loader api.Loader
}

func bundle(outDir string, define map[string]string, inlines map[string]inline, entries ...string) (map[string][]byte, error) {
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
		Plugins: []api.Plugin{{
			Name: "inline",
			Setup: func(b api.PluginBuild) {
				b.OnResolve(api.OnResolveOptions{Filter: `^inline:`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					name := strings.TrimPrefix(args.Path, "inline:")
					if _, ok := inlines[name]; !ok {
						return api.OnResolveResult{}, fmt.Errorf("unknown inline import %q", name)
					}
					return api.OnResolveResult{Path: name, Namespace: "inline"}, nil
				})
				b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "inline"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					x := inlines[args.Path]
					contents := string(x.data)
					return api.OnLoadResult{Contents: &contents, Loader: x.loader}, nil
				})
			},
		}},
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
	return files, nil
}

func bundleWorker(entry string) ([]byte, error) {
	slog.Info("esbuild", "op", "bundle", "entries", []string{entry})
	res := api.Build(api.BuildOptions{
		EntryPoints:       []string{entry},
		Bundle:            true,
		Format:            api.FormatESModule,
		Target:            api.ES2022,
		Charset:           api.CharsetUTF8,
		Platform:          api.PlatformBrowser,
		External:          []string{"node:*"}, // kepubify's node fallback for loading the module
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
	if len(res.OutputFiles) != 1 {
		return nil, fmt.Errorf("expected one output file, got %d", len(res.OutputFiles))
	}
	return res.OutputFiles[0].Contents, nil
}

func hashAssets(dir string) (int, error) {
	fsys := os.DirFS(dir)
	hashes := map[string]string{}
	assetHash := func(name string) (string, error) {
		if h, ok := hashes[name]; ok {
			return h, nil
		}
		var h string
		if fi, err := fs.Stat(fsys, name); err == nil && fi.Mode().IsRegular() && path.Ext(name) != ".html" {
			b, err := fs.ReadFile(fsys, name)
			if err != nil {
				return "", err
			}
			sum := sha256.Sum256(b)
			h = hex.EncodeToString(sum[:6])
		}
		hashes[name] = h
		return h, nil
	}
	var n int
	err := fs.WalkDir(fsys, ".", func(page string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(page) != ".html" {
			return err
		}
		src, err := fs.ReadFile(fsys, page)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		x := htmlstream.NewTransformer(&buf, bytes.NewReader(src))
		for x.Next() {
			if x.Token().Kind() != htmlstream.StartTag {
				continue
			}
			for _, attr := range []string{"src", "href"} {
				a, ok := x.Token().Attr(attr)
				if !ok {
					continue
				}
				v := a.Value()
				u, err := url.Parse(v)
				if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") || u.RawQuery != "" || u.ForceQuery {
					continue // not a relative path, or already has a query
				}
				name := path.Join(path.Dir(page), u.Path)
				if !fs.ValidPath(name) {
					continue
				}
				h, err := assetHash(name)
				if err != nil {
					return fmt.Errorf("%s: %w", page, err)
				}
				if h == "" {
					continue
				}
				v, frag, _ := strings.Cut(v, "#")
				v += "?v=" + h
				if u.Fragment != "" {
					v += "#" + frag
				}
				if err := x.SetAttr(attr, v); err != nil {
					return fmt.Errorf("%s: %w", page, err)
				}
				n++
			}
		}
		if err := x.Close(); err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}
		return os.WriteFile(filepath.Join(dir, filepath.FromSlash(page)), buf.Bytes(), 0o644)
	})
	return n, err
}

// TODO: process CSS?
// TODO: html tweaks?
