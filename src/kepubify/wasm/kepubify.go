package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"syscall/js"
	_ "unsafe"

	"github.com/pgaskin/kepubify/_/go116-zip.go117/archive/zip"
	"github.com/pgaskin/kepubify/v4/kepub"
)

//go:generate env GOOS=js GOARCH=wasm go build -tags zip117 -trimpath -o kepubify.wasm

//go:linkname withProgress github.com/pgaskin/kepubify/v4/kepub.withProgress
func withProgress(ctx context.Context, delta float64, fn func(n, total int)) context.Context

type object = map[string]interface{}

func version() string {
	const major = "v4"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/pgaskin/kepubify/"+major {
				if dep.Replace != nil {
					if dep.Replace.Version != "" {
						return dep.Replace.Version
					}
					return dep.Version + " (dev)"
				}
				return dep.Version
			}
		}
	}
	return major
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			postMessage(object{
				"type":    "panic",
				"message": fmt.Sprint(err),
				"stack":   string(debug.Stack()),
			}, nil)
		} else {
			postMessage(object{
				"type": "exit",
			}, nil)
		}
	}()

	var (
		version = version()
	)

	postMessage(object{
		"type":    "version",
		"version": version,
	}, nil)

	if len(os.Args) != 2 {
		fmt.Println("[worker/"+os.Args[0]+"]", "kepubify", version)

		postMessage(object{
			"type":    "error",
			"message": "argument 1: missing init variable",
		}, nil)

		return
	}

	var (
		input    = os.Args[1]
		init     = Primitive(js.Global().Get(input)).Check("globalThis."+input, js.TypeObject, false, false).JSValue()
		epub     = ArrayBuffer(init.Get("epub")).Check("globalThis." + input + ".epub").ToUint8Array()
		config   = Array(init.Get("config")).Check("globalThis." + input + ".config")
		progress = Primitive(init.Get("progress")).Check("globalThis."+input+".progress", js.TypeBoolean, false, false).JSValue().Bool()
	)

	var opts []kepub.ConverterOption
	for _, x := range config.Items() {
		Primitive(x).Check("globalThis."+input+".config[]", js.TypeObject, false, false).JSValue()
		switch t := Primitive(x.Get("type")).Check("globalThis."+input+".config[].type", js.TypeString, false, false).JSValue().String(); t {
		case "Smartypants":
			opts = append(opts, kepub.ConverterOptionSmartypants())
		case "FindReplace":
			opts = append(opts, kepub.ConverterOptionFindReplace(
				Primitive(x.Get("find")).Check("globalThis."+input+".config[]<"+t+">.find", js.TypeString, false, false).JSValue().String(),
				Primitive(x.Get("replace")).Check("globalThis."+input+".config[]<"+t+">.replace", js.TypeString, false, false).JSValue().String(),
			))
		case "DummyTitlepage":
			opts = append(opts, kepub.ConverterOptionDummyTitlepage(
				Primitive(x.Get("add")).Check("globalThis."+input+".config[]<"+t+">.add", js.TypeBoolean, false, false).JSValue().Bool(),
			))
		case "AddCSS":
			opts = append(opts, kepub.ConverterOptionAddCSS(
				Primitive(x.Get("css")).Check("globalThis."+input+".config[]<"+t+">.css", js.TypeString, false, false).JSValue().String(),
			))
		case "Hyphenate":
			opts = append(opts, kepub.ConverterOptionHyphenate(
				Primitive(x.Get("hyphenate")).Check("globalThis."+input+".config[]<"+t+">.hyphenate", js.TypeBoolean, false, false).JSValue().Bool(),
			))
		case "FullScreenFixes":
			opts = append(opts, kepub.ConverterOptionFullScreenFixes())
		}
	}

	fmt.Println("[worker/"+os.Args[0]+"]", "converting", epub.Size(), "bytes with options:", len(opts), ", progress:", progress)

	var (
		converter = kepub.NewConverterWithOptions(opts...)
		ctx       = context.Background()
	)

	if progress {
		ctx = withProgress(ctx, 0.03, func(n, total int) {
			postMessage(object{
				"type":  "progress",
				"n":     n,
				"total": total,
			}, nil)
		})
	}

	zr, err := zip.NewReader(epub, epub.Size())
	if err != nil {
		postMessage(object{
			"type":    "error",
			"message": "read EPUB: " + err.Error(),
		}, nil)
		return
	}

	buf := bytes.NewBuffer(make([]byte, 0, epub.Size()))
	if err := converter.Convert(ctx, buf, zr); err != nil {
		postMessage(object{
			"type":    "error",
			"message": "convert EPUB: " + err.Error(),
		}, nil)
		return
	}

	arr := NewUint8Array(buf.Bytes()).ToArrayBuffer()

	fmt.Println("[kepubify/"+os.Args[0]+"]", "done")

	postMessage(object{
		"type":  "success",
		"kepub": arr,
	}, []interface{}{
		arr,
	})
}
