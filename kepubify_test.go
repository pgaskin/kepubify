package main

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
	assert.False(t, isDir("./kepubify_test.go"), "./kepubify_test.go should not be a dir")
}

func TestIsFile(t *testing.T) {
	assert.False(t, isFile("."), ". should not be a file")
	assert.False(t, isFile("sdfsdfsdf"), "sdfsdfsdf should not be a file")
	assert.True(t, isFile("kepubify.go"), "kepubify.go should be a file")
}
