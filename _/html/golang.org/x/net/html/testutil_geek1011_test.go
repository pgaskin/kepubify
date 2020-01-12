// Copyright 2020 Patrick Gaskin.
package html

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

type testModCase struct {
	What     string
	Original string

	ParseOptsA  []ParseOption
	RenderOptsA []RenderOption
	RenderedA   string

	ParseOptsB  []ParseOption
	RenderOptsB []RenderOption
	RenderedB   string
}

func (tc testModCase) Test(t *testing.T) {
	t.Logf("case %#v", tc.What)
	outA, err := testModRerender(tc.Original, tc.ParseOptsA, tc.RenderOptsA, true)
	if err != nil {
		t.Errorf("case %#v: version A: %v", tc.What, err)
	} else if outA != tc.RenderedA {
		t.Errorf("case %#v: version A: unexpected got:`%s` != expected:`%s`", tc.What, outA, tc.RenderedA)
	}

	outB, err := testModRerender(tc.Original, tc.ParseOptsB, tc.RenderOptsB, true)
	if err != nil {
		t.Errorf("case %#v: version B: %v", tc.What, err)
	} else if outB != tc.RenderedB {
		t.Errorf("case %#v: version B: unexpected got:`%s` != expected:`%s`", tc.What, outB, tc.RenderedB)
	}
}

func testModRerender(orig string, popts []ParseOption, ropts []RenderOption, consistency bool) (string, error) {
	node, err := ParseWithOptions(strings.NewReader(orig), popts...)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := RenderWithOptions(buf, node, ropts...); err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	out := buf.String()

	if consistency {
		rpNode, err := ParseWithOptions(strings.NewReader(out), popts...)
		if err != nil {
			return "", fmt.Errorf("reparse rendered: %w", err)
		}

		buf.Reset()
		if err := RenderWithOptions(buf, rpNode, ropts...); err != nil {
			return "", fmt.Errorf("rerender reparsed rendered: %w", err)
		}
		rpOut := buf.String()

		if out != rpOut {
			return "", fmt.Errorf("inconsistent rerendering with same opts: first:%#v != second:%#v", out, rpOut)
		}
	}

	return out, nil
}
