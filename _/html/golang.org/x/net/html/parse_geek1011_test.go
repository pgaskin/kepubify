// Copyright 2020 Patrick Gaskin.

package html

import (
	"testing"
)

func TestMod_ParseLenientSelfClosing(t *testing.T) {
	testModCase{
		What:     `Self closing title in head`,
		Original: `<!DOCTYPE html><html><head><title/></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>&lt;/head&gt;&lt;body&gt;&lt;p&gt;Test 1&lt;/p&gt;&lt;/body&gt;&lt;/html&gt;</title></head><body></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title></title></head><body><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing title in head with elements around`,
		Original: `<!DOCTYPE html><html><head><base href="/"/><title/><meta charset="asd"/></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><base href="/"/><title>&lt;meta charset=&#34;asd&#34;/&gt;&lt;/head&gt;&lt;body&gt;&lt;p&gt;Test 1&lt;/p&gt;&lt;/body&gt;&lt;/html&gt;</title></head><body></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><base href="/"/><title></title><meta charset="asd"/></head><body><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing title in body`,
		Original: `<!DOCTYPE html><html><head></head><body><title/><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head></head><body><title>&lt;p&gt;Test 1&lt;/p&gt;&lt;/body&gt;&lt;/html&gt;</title></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head></head><body><title></title><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing script in head`,
		Original: `<!DOCTYPE html><html><head><title>Title</title><script src="script.js"/></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title><script src="script.js"></head><body><p>Test 1</p></body></html></script></head><body></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title><script src="script.js"></script></head><body><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing script in head with elements around`,
		Original: `<!DOCTYPE html><html><head><title>Title</title><base href="/"/><script src="script.js"/><meta charset="asd"/></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title><base href="/"/><script src="script.js"><meta charset="asd"/></head><body><p>Test 1</p></body></html></script></head><body></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title><base href="/"/><script src="script.js"></script><meta charset="asd"/></head><body><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing script in body`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><script src="script.js"/><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><script src="script.js"><p>Test 1</p></body></html></script></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><script src="script.js"></script><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing a in body (simple)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"/> 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"> 1</a></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"></a> 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing a in body (multiple)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"/><a id="test1"/> 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"></a><a id="test1"> 1</a></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <a id="test"></a><a id="test1"></a> 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing a in body (within text and escaped characters around multiple elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<a id="test"/>&gt;&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<a id="test">&gt;&lt; 1<span>test</span></a></p><p><a id="test">Test 2</a></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<a id="test"></a>&gt;&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing a in body (within text and escaped characters around multiple elements + HTML5 unclosed formatting elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<a id="test"/>&gt;<b>&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<a id="test">&gt;<b>&lt; 1<span>test</span></b></a></i></p><p><i><a id="test"><b>Test 2</b></a></i></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<a id="test"></a>&gt;<b>&lt; 1<span>test</span></b></i></p><p><i><b>Test 2</b></i></p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing span in body (simple)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"/> 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"> 1</span></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"></span> 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing span in body (multiple)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"/><span id="test1"/> 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"><span id="test1"> 1</span></span></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test <span id="test"></span><span id="test1"></span> 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing span in body (within text and escaped characters around multiple elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<span id="test" />&gt;&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<span id="test">&gt;&lt; 1<span>test</span></span></p><p>Test 2</p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test &gt;&lt;<span id="test"></span>&gt;&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing span in body (within text and escaped characters around multiple elements + HTML5 unclosed formatting elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<span id="test" />&gt;<b>&lt; 1<span>test</span></p><p>Test 2</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<span id="test">&gt;<b>&lt; 1<span>test</span></b></span></i></p><p><i><b>Test 2</b></i></p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p><i>Test &gt;&lt;<span id="test"></span>&gt;<b>&lt; 1<span>test</span></b></i></p><p><i><b>Test 2</b></i></p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing p in body (simple, the other funny cases are basically already tested through the other elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><p/><span>Test</span><p>Test</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><p><span>Test</span></p><p>Test</p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><p></p><span>Test</span><p>Test</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `Self closing div in body (simple, the other funny cases are basically already tested through the other elements)`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><div/><span>Test</span><p>Test</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(false)},
		RenderOptsA: nil,
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><div><span>Test</span><p>Test</p></div></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: nil,
		RenderedB:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p>Test</p><span>Test</span><div></div><span>Test</span><p>Test</p></body></html>`,
	}.Test(t)
}

func TestMod_ParseIgnoreBOM(t *testing.T) {
	testModCase{
		What:     `BOM and comment`,
		Original: "\xEF\xBB\xBF" + `<!-- Comment Text --><!DOCTYPE html><html><head><title>Title</title></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionIgnoreBOM(false)},
		RenderOptsA: nil,
		RenderedA:   `<html><head></head><body>` + "\xEF\xBB\xBF" + `<!-- Comment Text --><title>Title</title><p>Test 1</p></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionIgnoreBOM(true)},
		RenderOptsB: nil,
		RenderedB:   `<!-- Comment Text --><!DOCTYPE html><html><head><title>Title</title></head><body><p>Test 1</p></body></html>`,
	}.Test(t)
}
