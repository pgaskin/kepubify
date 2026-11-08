//go:build wasip1

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"unsafe"

	"github.com/pgaskin/kepubify/v5/kepub"
)

func main() {}

// live holds the buffers returned by alloc and the last result, since the host
// only has the addresses.
var live [][]byte

func addr(b []byte) uint32 { return uint32(uintptr(unsafe.Pointer(unsafe.SliceData(b)))) }

func view(p, n uint32) []byte {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Add(nil, p)), n)
}

//go:wasmimport kepubify warn
func hostWarn(name, nameLen, msg, msgLen uint32)

//go:wasmimport kepubify progress
func hostProgress(percent float64)

//go:wasmexport alloc
func alloc(n uint32) uint32 {
	b := make([]byte, max(n, 1))
	live = append(live, b)
	return addr(b)
}

//go:wasmexport convert
func convert(in, inLen, opts, optsLen uint32) uint32 {
	opt := options(view(opts, optsLen))
	input := view(in, inLen)
	zr, err := zip.NewReader(bytes.NewReader(input), int64(len(input)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return result(1, []byte(err.Error()))
	}
	var buf bytes.Buffer
	if err := kepub.Convert(context.Background(), &buf, zr, opt); errors.Is(err, kepub.ErrAlreadyKepub) {
		return result(2, []byte(err.Error()))
	} else if err != nil {
		return result(1, []byte(err.Error()))
	}
	return result(0, buf.Bytes())
}

//go:wasmexport transform_content
func transformContent(in, inLen, opts, optsLen uint32) uint32 {
	opt := options(view(opts, optsLen))
	var buf bytes.Buffer
	if err := kepub.TransformContent(&buf, bytes.NewReader(view(in, inLen)), opt); err != nil {
		return result(1, []byte(err.Error()))
	}
	return result(0, buf.Bytes())
}

func result(status uint32, b []byte) uint32 {
	r := make([]byte, 12)
	for i, v := range []uint32{status, addr(b), uint32(len(b))} {
		r[i*4], r[i*4+1], r[i*4+2], r[i*4+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	}
	live = append(live, b, r)
	return addr(r)
}

func options(o []byte) kepub.Options {
	r := optionReader(o)
	flags := r.byte()
	hyphenate, dummyTitlepage := r.byte(), r.byte()

	var opt kepub.Options
	opt.Workers = 1
	opt.FullScreenFixes = flags&1 != 0
	opt.Smartypants = flags&2 != 0
	opt.DetectCover = flags&4 != 0
	opt.ReprocessKepub = flags&8 != 0
	if flags&16 != 0 {
		opt.Warn = func(name, msg string) {
			n, m := []byte(name), []byte(msg)
			hostWarn(addr(n), uint32(len(n)), addr(m), uint32(len(m)))
		}
	}
	if flags&32 != 0 {
		opt.Progress = func(percent float64) {
			hostProgress(percent) // note: TinyGo doesn't allow imported functions as values
		}
	}
	for _, x := range []struct {
		v byte
		p **bool
	}{{hyphenate, &opt.Hyphenate}, {dummyTitlepage, &opt.DummyTitlepage}} {
		switch x.v {
		case 1:
			*x.p = new(true)
		case 2:
			*x.p = new(false)
		}
	}
	opt.Charset = r.string()
	for range r.int() {
		opt.CSS = append(opt.CSS, r.string())
	}
	for range r.int() {
		opt.Replace = append(opt.Replace, kepub.Replacement{Find: r.string(), Replace: r.string()})
	}
	return opt
}

type optionReader []byte

func (r *optionReader) byte() (v byte) {
	if len(*r) > 0 {
		v, *r = (*r)[0], (*r)[1:]
	}
	return
}

func (r *optionReader) int() int {
	b := *r
	if len(b) < 4 {
		*r = nil
		return 0
	}
	*r = b[4:]
	return int(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

func (r *optionReader) string() string {
	n := min(r.int(), len(*r))
	s := string((*r)[:n])
	*r = (*r)[n:]
	return s
}
