package utils

import (
	"io"
	"unicode/utf8"
)

const (
	surrSelf = 0x10000

	// bufferSize is the number of bytes buffered by the byte-string encoder and decoder.
	bufferSize = 1024
)

func Utf16Len(v rune) int {
	if surrSelf <= v && v <= utf8.MaxRune {
		return 2
	}
	return 1
}

func Utf16Count(src []byte) (n int) {
	for len(src) > 0 {
		if src[0] < utf8.RuneSelf {
			n++
			src = src[1:]
			continue
		}
		r, size := utf8.DecodeRune(src)
		n += Utf16Len(r)
		src = src[size:]
	}
	return
}

func Utf16CountString(src string) (n int) {
	for _, rb := range src {
		n += Utf16Len(rb)
	}
	return
}

func Utf8encodeString(src string) string {
	if len(src) == 0 {
		return ""
	}

	buf := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		rb := rune(src[i])
		buf = utf8.AppendRune(buf, rb)
	}
	return string(buf)
}

func Utf8encodeBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	buf := make([]byte, 0, len(src))
	for _, b := range src {
		rb := rune(b)
		buf = utf8.AppendRune(buf, rb)
	}
	return buf
}

func Utf8decodeString(byteString string) string {
	if len(byteString) == 0 {
		return ""
	}

	buf := make([]byte, 0, len(byteString))
	for _, rb := range byteString {
		buf = append(buf, byte(rb))
	}
	return string(buf)
}

func Utf8decodeBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	buf := make([]byte, 0, len(src))
	for len(src) > 0 {
		r, l := utf8.DecodeRune(src)
		src = src[l:]
		buf = append(buf, byte(r))
	}
	return buf
}

func utf8encodeBytes(dst, src []byte) int {
	ndst := 0
	for _, b := range src {
		rb := rune(b)
		n := utf8.EncodeRune(dst[ndst:], rb)
		ndst += n
	}
	return ndst
}

func utf8decodeBytes(dst, src []byte, atEOF bool) (ndst, nsrc int) {
	for len(src) > 0 {
		if ndst >= len(dst) || (!atEOF && !utf8.FullRune(src)) {
			break
		}
		r, l := utf8.DecodeRune(src)
		src = src[l:]
		dst[ndst] = byte(r)
		nsrc += l
		ndst++
	}
	return
}

type utf8encoder struct {
	w   io.Writer
	err error
	out [bufferSize]byte // output buffer
}

// NewUtf8Encoder writes each input byte as its corresponding Unicode code point in UTF-8.
func NewUtf8Encoder(w io.Writer) io.Writer {
	return &utf8encoder{w: w}
}

func (e *utf8encoder) Write(p []byte) (n int, err error) {
	for len(p) > 0 && e.err == nil {
		chunkSize := min(len(p), bufferSize/2)

		encoded := utf8encodeBytes(e.out[:], p[:chunkSize])
		_, e.err = e.w.Write(e.out[:encoded])
		n += chunkSize
		p = p[chunkSize:]
	}
	return n, e.err
}

type utf8decoder struct {
	r     io.Reader
	err   error
	buf   [bufferSize]byte
	start int
	end   int
}

func NewUtf8Decoder(r io.Reader) io.Reader {
	return &utf8decoder{r: r}
}

func (d *utf8decoder) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for {
		if d.start < d.end {
			var consumed int
			n, consumed = utf8decodeBytes(p, d.buf[d.start:d.end], d.err != nil)
			d.start += consumed
			if n > 0 {
				return n, nil
			}
		}

		// Deliver the source error only after all buffered input is decoded.
		if d.err != nil {
			return 0, d.err
		}

		// Only an incomplete rune can remain here; retain it for the next read.
		d.end = copy(d.buf[:], d.buf[d.start:d.end])
		d.start = 0
		var read int
		read, d.err = d.r.Read(d.buf[d.end:])
		d.end += read
	}
}
