package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKepubify(t *testing.T) {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		assert.FailNow(t, fmt.Sprintf("%v", err), "Could not create temp dir")
	}
	defer os.RemoveAll(td)

	wd, err := os.Getwd()
	if err != nil {
		assert.FailNow(t, fmt.Sprintf("%v", err), "Could not get current dir")
	}

	kepub := filepath.Join(td, "test1.kepub.epub")
	kepubunp := filepath.Join(td, "test1.kepub.epub_unpacked")

	err = Kepubify(filepath.Join(wd, "testdata", "books", "test1.epub"), kepub, false, nil, nil)
	assert.Nil(t, err, "should not error when converting book")
	assert.True(t, exists(kepub), "converted kepub should exist")

	assert.Nil(t, UnpackEPUB(kepub, kepubunp, true), "should not error when unpacking converted kepub")
	assert.True(t, exists(kepubunp), "unpacked kepub dir should exist")
	assert.True(t, exists(filepath.Join(kepubunp, "META-INF", "container.xml")), "kepub should have a container.xml")
}
