package utils

import (
	"io"
	"unicode/utf8"
)

const (
	maxRune  = '\U0010FFFF'
	surr1    = 0xd800
	surr3    = 0xe000
	surrSelf = 0x10000

	// bufferSize is the number of hexadecimal characters to buffer in encoder and decoder.
	bufferSize = 1024
)

func Utf16Len(v rune) int {
	if (0 <= v && v < surr1) || (surr3 <= v && v < surrSelf) {
		return 1
	}
	if surrSelf <= v && v <= maxRune {
		return 2
	}
	return 1
}

func Utf16Count(src []byte) (n int) {
	for len(src) > 0 {
		rb, l := utf8.DecodeRune(src)
		src = src[l:]
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
		if (0 <= rb && rb < surr1) || (surr3 <= rb && rb < surrSelf) {
			n++
		} else if surrSelf <= rb && rb <= maxRune {
			n += 2
		} else {
			n++
		}
	}
	return
}

func Utf16CountString(src string) (n int) {
	for _, rb := range src {
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
		if (0 <= rb && rb < surr1) || (surr3 <= rb && rb < surrSelf) {
			n++
		} else if surrSelf <= rb && rb <= maxRune {
			n += 2
		} else {
			n++
		}
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
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
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
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
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
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
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
		if !utf8.ValidRune(r) {
			r = utf8.RuneError
		}
		buf = append(buf, byte(r))
	}
	return buf
}

func utf8encodeBytes(dst, src []byte) int {
	ndst := 0
	for _, b := range src {
		rb := rune(b)
		if !utf8.ValidRune(rb) {
			rb = utf8.RuneError
		}
		n := utf8.EncodeRune(dst[ndst:], rb)
		ndst += n
	}
	return ndst
}

func utf8decodeBytes(dst, src []byte) (ndst, nsrc int, err error) {
	for len(src) > 0 {
		r, l := utf8.DecodeRune(src)
		src = src[l:]
		if !utf8.ValidRune(r) {
			r = utf8.RuneError
		}
		if ndst >= len(dst) {
			break
		}
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

// NewEncoder returns an io.Writer that writes lowercase hexadecimal characters to w.
func NewUtf8Encoder(w io.Writer) io.Writer {
	return &utf8encoder{w: w}
}

func (e *utf8encoder) Write(p []byte) (n int, err error) {
	for len(p) > 0 && e.err == nil {
		chunkSize := bufferSize / 2
		if len(p) < chunkSize {
			chunkSize = len(p)
		}

		encoded := utf8encodeBytes(e.out[:], p[:chunkSize])
		_, e.err = e.w.Write(e.out[:encoded])
		n += chunkSize
		p = p[chunkSize:]
	}
	return n, e.err
}

type utf8decoder struct {
	err     error
	readErr error
	r       io.Reader
	buf     [bufferSize]byte // leftover input
	nbuf    int
	out     []byte // leftover decoded output
	outbuf  [bufferSize]byte
}

func NewUtf8Decoder(r io.Reader) io.Reader {
	return &utf8decoder{r: r}
}

func (d *utf8decoder) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	if d.err != nil {
		return 0, d.err
	}

	for {
		// Copy leftover output from last decode.
		if len(d.out) > 0 {
			n = copy(p, d.out)
			d.out = d.out[n:]
			return n, nil
		}

		// Decode leftover input from last read.
		var nn, nsrc, ndst int
		if d.nbuf > 0 {
			ndst, nsrc, d.err = utf8decodeBytes(d.outbuf[0:], d.buf[0:d.nbuf])
			if ndst > 0 {
				d.out = d.outbuf[0:ndst]
				d.nbuf = copy(d.buf[0:], d.buf[nsrc:d.nbuf])
				continue // copy out and return
			}
		}

		// Out of input, out of decoded output. Check errors.
		if d.err != nil {
			return 0, d.err
		}
		if d.readErr != nil {
			d.err = d.readErr
			return 0, d.err
		}

		// Read more data.
		nn, d.readErr = d.r.Read(d.buf[d.nbuf:])
		d.nbuf += nn
	}
}
