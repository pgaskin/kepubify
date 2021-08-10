//go:build go1.16 && zip117
// +build go1.16,zip117

package zip

import "github.com/pgaskin/kepubify/_/go116-zip.go117/archive/zip"

const Deflate = zip.Deflate
const Store = zip.Store

var FileInfoHeader = zip.FileInfoHeader
var NewReader = zip.NewReader
var NewWriter = zip.NewWriter
var OpenReader = zip.OpenReader
var RegisterCompressor = zip.RegisterCompressor
var RegisterDecompressor = zip.RegisterDecompressor

type Compressor = zip.Compressor
type Decompressor = zip.Decompressor
type File = zip.File
type FileHeader = zip.FileHeader
type ReadCloser = zip.ReadCloser
type Reader = zip.Reader
type Writer = zip.Writer

var ErrAlgorithm = zip.ErrAlgorithm
var ErrChecksum = zip.ErrChecksum
var ErrFormat = zip.ErrFormat
