//go:build !wasip1

package main

import (
	"flag"
	"fmt"
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

const outDir = "dist"

var toolchain = flag.String("toolchain", "auto", "toolchain for the wasm module (auto, tinygo, go)")

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	flag.Parse()
	if _, err := os.Stat(filepath.FromSlash("src/kepubify.ts")); err != nil {
		fatal("this script must be run from the js directory", err)
	}
	if _, err := os.Stat("node_modules"); err != nil {
		fatal("npm packages not installed, run npm ci", err)
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
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	license, err := os.ReadFile(filepath.FromSlash("../LICENSE"))
	if err != nil {
		return err
	}
	if err := os.WriteFile("LICENSE", license, 0o644); err != nil {
		return err
	}

	wasm, used, err := buildWasm(filepath.Join(outDir, "kepubify.wasm"))
	if err != nil {
		return err
	}

	js, err := bundle(outDir, "src/index.ts", "src/worker.ts", "src/kepubify.worker.ts")
	if err != nil {
		return err
	}
	for name, data := range js {
		if err := os.WriteFile(filepath.Join(outDir, name), data, 0o644); err != nil {
			return err
		}
	}

	if err := run(exec.Command(filepath.FromSlash("node_modules/.bin/tsc"), "-p", "tsconfig.json")); err != nil {
		return fmt.Errorf("tsc: %w", err)
	}

	for _, name := range slices.Sorted(maps.Keys(js)) {
		fmt.Printf("%-24s %8d bytes\n", name, len(js[name]))
	}
	fmt.Printf("%-24s %8d bytes (%s)\n", "kepubify.wasm", len(wasm), used)
	return nil
}

func buildWasm(out string) ([]byte, string, error) {
	tool := *toolchain
	switch tool {
	case "auto":
		tool = "go"
		if _, err := exec.LookPath("tinygo"); err == nil {
			tool = "tinygo"
		}
	case "tinygo", "go":
	default:
		return nil, "", fmt.Errorf("unknown toolchain %q", tool)
	}

	var cmd *exec.Cmd
	switch tool {
	case "tinygo":
		cmd = exec.Command("tinygo", "build", "-target", "wasip1", "-buildmode", "c-shared",
			"-scheduler", "none", "-gc", "precise", "-opt", "z", "-no-debug",
			"-o", out, "./src")
	case "go":
		cmd = exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-ldflags=-s", "-gcflags=all=-B",
			"-o", out, "./src")
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	}
	if err := run(cmd); err != nil {
		return nil, "", fmt.Errorf("build wasm with %s: %w", tool, err)
	}
	used := tool

	if path, err := exec.LookPath("wasm-opt"); err == nil {
		cmd := exec.Command(path, "-Oz", "--enable-bulk-memory", "--enable-nontrapping-float-to-int",
			"--enable-sign-ext", "--enable-mutable-globals", out, "-o", out)
		if err := run(cmd); err != nil {
			return nil, "", fmt.Errorf("wasm-opt: %w", err)
		}
		used += " + wasm-opt"
	}

	wasm, err := os.ReadFile(out)
	return wasm, used, err
}

func bundle(outDir string, entries ...string) (map[string][]byte, error) {
	slog.Info("esbuild", "op", "bundle", "entries", entries)
	res := api.Build(api.BuildOptions{
		EntryPoints:   entries,
		Outdir:        outDir,
		Bundle:        true,
		Splitting:     true,
		Format:        api.FormatESModule,
		Target:        api.ES2022,
		Charset:       api.CharsetUTF8,
		Platform:      api.PlatformNeutral,
		External:      []string{"node:*"},
		TreeShaking:   api.TreeShakingTrue,
		LegalComments: api.LegalCommentsNone,
		LogLevel:      api.LogLevelSilent,
		Write:         false,
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
