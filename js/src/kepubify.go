//go:build wasip1

package main

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"io"
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

//go:wasmimport kepubify write
func hostWrite(p, n uint32)

//go:wasmimport kepubify read
func hostRead(p, n uint32, off float64) uint32

//go:wasmexport alloc
func alloc(n uint32) uint32 {
	b := make([]byte, max(n, 1))
	live = append(live, b)
	return addr(b)
}

//go:wasmexport convert
func convert(size float64, opts, optsLen uint32) uint32 {
	opt := options(view(opts, optsLen))
	zr, err := zip.NewReader(newHostReader(int64(size)), int64(size))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return result(1, []byte(err.Error()))
	}
	w := newHostWriter()
	if err := kepub.Convert(context.Background(), w, zr, opt); errors.Is(err, kepub.ErrAlreadyKepub) {
		return result(2, []byte(err.Error()))
	} else if err != nil {
		return result(1, []byte(err.Error()))
	}
	if err := w.Flush(); err != nil {
		return result(1, []byte(err.Error()))
	}
	return result(0, nil)
}

//go:wasmexport transform_content
func transformContent(size float64, opts, optsLen uint32) uint32 {
	opt := options(view(opts, optsLen))
	w := newHostWriter()
	if err := kepub.TransformContent(w, io.NewSectionReader(newHostReader(int64(size)), 0, int64(size)), opt); err != nil {
		return result(1, []byte(err.Error()))
	}
	if err := w.Flush(); err != nil {
		return result(1, []byte(err.Error()))
	}
	return result(0, nil)
}

func newHostWriter() *bufio.Writer {
	return bufio.NewWriterSize(hostWriter{}, 64<<10)
}

func newHostReader(size int64) *hostReader {
	return &hostReader{size: size, buf: make([]byte, min(size, 1<<20))} // 1 MB readahead
}

type hostWriter struct{}

func (hostWriter) Write(b []byte) (int, error) {
	if len(b) != 0 {
		hostWrite(addr(b), uint32(len(b)))
	}
	return len(b), nil
}

type hostReader struct {
	size int64
	buf  []byte // readahead if read is smaller (much better performance since compress/flate does a lot of small reads)
	off  int64  // of buf
	n    int    // valid bytes in buf
}

func (r *hostReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= r.size {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p = p[:min(int64(len(p)), r.size-off)]
	var n int
	switch {
	case off >= r.off && off+int64(len(p)) <= r.off+int64(r.n):
		n = copy(p, r.buf[off-r.off:r.n])
	case len(p) >= len(r.buf):
		n = hostReadAt(p, off)
	default:
		r.off, r.n = off, hostReadAt(r.buf[:min(int64(len(r.buf)), r.size-off)], off)
		n = copy(p, r.buf[:r.n])
	}
	if n < len(p) || off+int64(n) == r.size {
		return n, io.EOF
	}
	return n, nil
}

func hostReadAt(p []byte, off int64) int {
	return int(hostRead(addr(p), uint32(len(p)), float64(off)))
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
