// Copyright 2020 Patrick Gaskin.

package html

import "testing"

func TestMod_RenderAllowXMLDeclarations(t *testing.T) {
	testModCase{
		What:     `Standard XML declaration (keep)`,
		Original: `<?xml version='1.0' encoding='utf-8'?><html><head><title>Title</title></head><body><p>Test 1</p></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionAllowXMLDeclarations(false)},
		RenderedA:   `<!--?xml version='1.0' encoding='utf-8'?--><html><head><title>Title</title></head><body><p>Test 1</p></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionAllowXMLDeclarations(true)},
		RenderedB:   `<?xml version='1.0' encoding='utf-8'?><html><head><title>Title</title></head><body><p>Test 1</p></body></html>`,
	}.Test(t)

	testModCase{
		What:     `XML procesing instruction (ignore)`,
		Original: `<?xml version='1.0' encoding='utf-8'?><html><head><title>Title</title></head><body><?xml-stylesheet type="text/xsl" href="style.xsl"?><p>Test 1</p></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionAllowXMLDeclarations(false)},
		RenderedA:   `<!--?xml version='1.0' encoding='utf-8'?--><html><head><title>Title</title></head><body><!--?xml-stylesheet type="text/xsl" href="style.xsl"?--><p>Test 1</p></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionAllowXMLDeclarations(true)},
		RenderedB:   `<?xml version='1.0' encoding='utf-8'?><html><head><title>Title</title></head><body><!--?xml-stylesheet type="text/xsl" href="style.xsl"?--><p>Test 1</p></body></html>`,
	}.Test(t)
}

func TestMod_RenderPolyglot(t *testing.T) {
	testModCase{
		What:     `[mod] Use &#160; for NBSPs`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body>` + "\u00a0" + `&nbsp;&NonBreakingSpace;&#x000A0;&#160;</body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body>` + "\u00a0\u00a0\u00a0\u00a0\u00a0" + `</body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body>&#160;&#160;&#160;&#160;&#160;</body></html>`,
	}.Test(t)

	testModCase{
		What:     `[mod] Add xmlns to html`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[mod] Add xmlns to math and svg`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><math></math><svg></svg></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><math></math><svg></svg></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body><math xmlns="http://www.w3.org/1998/Math/MathML"></math><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"></svg></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[mod] Add type to script and style`,
		Original: `<!DOCTYPE html><html><head><title>Title</title><style></style><script></script></head><body></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title><style></style><script></script></head><body></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title><style type="text/css"></style><script type="text/javascript"></script></head><body></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Value for boolean attributes`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><input type="text" enabled/></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><input type="text" enabled=""/></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body><input type="text" enabled=""/></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Always add /> to void elements`,
		Original: `<!DOCTYPE html><html><head><title>Title</title><meta></head><body><input><img><br><wbr><hr></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title><meta/></head><body><input/><img/><br/><wbr/><hr/></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title><meta/></head><body><input/><img/><br/><wbr/><hr/></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Never self-close non-void elements`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><p/><div/><a/><span/></body></html>`,

		ParseOptsA:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><p></p><div></div><a></a><span></span></body></html>`,

		ParseOptsB:  []ParseOption{ParseOptionLenientSelfClosing(true)},
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body><p></p><div></div><a></a><span></span></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Only use named escapes for <>&`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body>&lt;&gt;&amp;&nbsp;&raquo;</body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body>&lt;&gt;&amp;` + "\u00a0" + `»</body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body>&lt;&gt;&amp;&#160;»</body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Only use <!-- and --> for comments`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><!-- Comment 1 --><! Comment 2 ></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><!-- Comment 1 --><!-- Comment 2 --></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body><!-- Comment 1 --><!-- Comment 2 --></body></html>`,
	}.Test(t)

	testModCase{
		What:     `[default] Wrap table contents in <tbody>`,
		Original: `<!DOCTYPE html><html><head><title>Title</title></head><body><table><tr><td>test</td></tr><tr><td>test</td></tr></table></body></html>`,

		ParseOptsA:  nil,
		RenderOptsA: []RenderOption{RenderOptionPolyglot(false)},
		RenderedA:   `<!DOCTYPE html><html><head><title>Title</title></head><body><table><tbody><tr><td>test</td></tr><tr><td>test</td></tr></tbody></table></body></html>`,

		ParseOptsB:  nil,
		RenderOptsB: []RenderOption{RenderOptionPolyglot(true)},
		RenderedB:   `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Title</title></head><body><table><tbody><tr><td>test</td></tr><tr><td>test</td></tr></tbody></table></body></html>`,
	}.Test(t)
}
