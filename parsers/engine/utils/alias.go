package utils

import (
	"io"

	"github.com/zishang520/socket.io/v3/utils"
)

// Alias  [utils.Utf16Len]
//
// Deprecated: will be removed in future versions, please use [utils.Utf16Len]
func Utf16Len(v rune) int {
	return utils.Utf16Len(v)
}

// Alias  [utils.Utf16Count]
//
// Deprecated: will be removed in future versions, please use [utils.Utf16Count]
func Utf16Count(src []byte) int {
	return utils.Utf16Count(src)
}

// Alias  [utils.Utf16CountString]
//
// Deprecated: will be removed in future versions, please use [utils.Utf16CountString]
func Utf16CountString(src string) int {
	return utils.Utf16CountString(src)
}

// Alias  [utils.Utf8encodeString]
//
// Deprecated: will be removed in future versions, please use [utils.Utf8encodeString]
func Utf8encodeString(src string) string {
	return utils.Utf8encodeString(src)
}

// Alias  [utils.Utf8encodeBytes]
//
// Deprecated: will be removed in future versions, please use [utils.Utf8encodeBytes]
func Utf8encodeBytes(src []byte) []byte {
	return utils.Utf8encodeBytes(src)
}

// Alias  [utils.Utf8decodeString]
//
// Deprecated: will be removed in future versions, please use [utils.Utf8decodeString]
func Utf8decodeString(byteString string) string {
	return utils.Utf8decodeString(byteString)
}

// Alias  [utils.Utf8decodeBytes]
//
// Deprecated: will be removed in future versions, please use [utils.Utf8decodeBytes]
func Utf8decodeBytes(src []byte) []byte {
	return utils.Utf8decodeBytes(src)
}

// Alias  [utils.NewUtf8Encoder]
//
// Deprecated: will be removed in future versions, please use [utils.NewUtf8Encoder]
func NewUtf8Encoder(w io.Writer) io.Writer {
	return utils.NewUtf8Encoder(w)
}

// Alias  [utils.NewUtf8Decoder]
//
// Deprecated: will be removed in future versions, please use [utils.NewUtf8Decoder]
func NewUtf8Decoder(r io.Reader) io.Reader {
	return utils.NewUtf8Decoder(r)
}
