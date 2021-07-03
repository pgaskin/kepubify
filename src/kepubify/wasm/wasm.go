package main

import (
	"errors"
	"os"
	"syscall/js"
)

type Primitive js.Value

func (n Primitive) Check(prefix string, t js.Type, optional, nullable bool) Primitive {
	if js.Value(n).IsUndefined() {
		if !optional {
			panic(prefix + ": null")
		}
	} else if js.Value(n).IsNull() {
		if !nullable {
			panic(prefix + ": undefined")
		}
	} else if js.Value(n).Type() != t {
		if optional {
			if nullable {
				panic(prefix + ": not undefined|null|" + t.String())
			} else {
				panic(prefix + ": not undefined|" + t.String())
			}
		} else {
			if nullable {
				panic(prefix + ": not null|" + t.String())
			} else {
				panic(prefix + ": not a " + t.String())
			}
		}
	}
	return n
}

func (n Primitive) JSValue() js.Value {
	return js.Value(n)
}

type Array js.Value

func (a Array) Check(prefix string) Array {
	if !constructor("Array").Call("isArray", a).Bool() {
		panic(prefix + ": not an array")
	}
	return a
}

func (a Array) Len() int {
	return js.Value(a).Length()
}

func (a Array) Items() []js.Value {
	it := make([]js.Value, a.Len())
	for i := range it {
		it[i] = js.Value(a).Index(i)
	}
	return it
}

func (a Array) JSValue() js.Value {
	return js.Value(a)
}

type ArrayBuffer js.Value

func (a ArrayBuffer) Check(prefix string) ArrayBuffer {
	if js.Value(a).IsNull() {
		panic(prefix + ": null")
	}
	if js.Value(a).IsUndefined() {
		panic(prefix + ": undefined")
	}
	if !js.Value(a).InstanceOf(constructor("ArrayBuffer")) {
		panic(prefix + ": not an instance of ArrayBuffer")
	}
	return a
}

func (a ArrayBuffer) Slice(start, end int) ArrayBuffer {
	return ArrayBuffer(js.Value(a).Call("slice", start, end))
}

func (a ArrayBuffer) ToUint8Array() Uint8Array {
	return Uint8Array(constructor("Uint8Array").New(a))
}

func (a ArrayBuffer) JSValue() js.Value {
	return js.Value(a)
}

type Uint8Array js.Value

func NewUint8Array(b []byte) Uint8Array {
	a := constructor("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return Uint8Array(a)
}

func (a Uint8Array) Check(prefix string) Uint8Array {
	if js.Value(a).IsNull() {
		panic(prefix + ": null")
	}
	if js.Value(a).IsUndefined() {
		panic(prefix + ": undefined")
	}
	if !js.Value(a).InstanceOf(constructor("Uint8Array")) {
		panic(prefix + ": not an instance of Uint8Array")
	}
	return a
}

func (a Uint8Array) ToArrayBuffer() ArrayBuffer {
	return ArrayBuffer(js.Value(a).Get("buffer")).Slice(int(a.Offset()), int(a.Size()+a.Offset()))
}

func (a Uint8Array) Offset() int64 {
	return int64(js.Value(a).Get("byteOffset").Int())
}

func (a Uint8Array) Size() int64 {
	return int64(js.Value(a).Get("byteLength").Int())
}

func (a Uint8Array) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, &os.PathError{Op: "readat", Path: "Uint8Array", Err: errors.New("negative offset")}
	}
	return js.CopyBytesToGo(p, js.Value(a).Call("subarray", int(off))), nil
}

func (a Uint8Array) JSValue() js.Value {
	return js.Value(a)
}

func postMessage(msg interface{}, transfer []interface{}) {
	js.Global().Call("postMessage", msg, transfer)
}

func constructor(t string) js.Value {
	if c := js.Global().Get(t); c.Type() != js.TypeFunction {
		panic(t + " is not defined")
	} else {
		return c
	}
}
