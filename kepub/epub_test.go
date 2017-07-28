package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	assert.True(t, exists("."), ". should exist")
	assert.False(t, exists("./nonexistent"), "./nonexistent should not exist")
}

func TestIsDir(t *testing.T) {
	assert.True(t, isDir("."), ". should be a dir")
	assert.False(t, isDir("./epub_test.go"), "./epub_test.go should not be a dir")
}

func TestPackUnpack(t *testing.T) {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		assert.FailNow(t, fmt.Sprintf("%v", err), "Could not create temp dir")
	}
	defer os.RemoveAll(td)

	assert.Nil(t, PackEPUB(filepath.Join("testdata", "books", "test1"), filepath.Join(td, "test1.epub"), true), "packepub should not return an error")
	assert.True(t, exists(filepath.Join(td, "test1.epub")), "output epub should exist")

	assert.Nil(t, UnpackEPUB(filepath.Join(td, "test1.epub"), filepath.Join(td, "test1"), true), "unpackepub should not return an error")
	assert.True(t, exists(filepath.Join(td, "test1")), "output dir should exist")
	assert.True(t, exists(filepath.Join(td, "test1", "META-INF", "container.xml")), "META-INF/container.xml should exist in output dir")

	assert.NotNil(t, PackEPUB(filepath.Join("testdata", "books", "invalid"), filepath.Join(td, "invalid.epub"), true), "packepub should return an error for an epub withot a container.xml")
}
