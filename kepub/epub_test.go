package kepub

import (
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
