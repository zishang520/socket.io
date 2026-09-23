package utils

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"
)

func TestUtf16InvalidRunes(t *testing.T) {
	for _, value := range []rune{-1, 0xd800, 0xdfff, utf8.MaxRune + 1} {
		if got := Utf16Len(value); got != 1 {
			t.Errorf("Utf16Len(%U) = %d, want 1", value, got)
		}
	}
	input := "A\xff\xf0\x9f\x99\x82\xc2"
	if got := Utf16Count([]byte(input)); got != 5 {
		t.Errorf("Utf16Count = %d, want 5", got)
	}
	if got := Utf16CountString(input); got != 5 {
		t.Errorf("Utf16CountString = %d, want 5", got)
	}
}

func TestUtf8DecoderReadBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  []byte
	}{
		{"empty", "", nil},
		{"code points", "A\u00e9\u4f60\U0001f642\ufffd", []byte{'A', 0xe9, 0x60, 0x42, 0xfd}},
		{"invalid bytes", "\xff\xc2A\xe4\xbd", []byte{0xfd, 0xfd, 'A', 0xfd, 0xfd}},
		{"multiple buffers", strings.Repeat("A\u00e9", bufferSize) + "\xe4\xbd", append(bytes.Repeat([]byte{'A', 0xe9}, bufferSize), 0xfd, 0xfd)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := Utf8decodeBytes([]byte(test.input)); !bytes.Equal(got, test.want) {
				t.Fatalf("Utf8decodeBytes = %x, want %x", got, test.want)
			}
			if got := Utf8decodeString(test.input); got != string(test.want) {
				t.Fatalf("Utf8decodeString = %x, want %x", got, test.want)
			}
			for _, size := range []int{1, 3, bufferSize + 1} {
				for _, source := range []io.Reader{
					strings.NewReader(test.input),
					iotest.OneByteReader(strings.NewReader(test.input)),
					iotest.DataErrReader(strings.NewReader(test.input)),
				} {
					decoder := NewUtf8Decoder(source)
					var got []byte
					buf := make([]byte, size)
					for {
						n, err := decoder.Read(buf)
						got = append(got, buf[:n]...)
						if err != nil {
							if err != io.EOF {
								t.Fatal(err)
							}
							break
						}
						if n == 0 {
							t.Fatal("decoder returned no data and no error")
						}
					}
					if !bytes.Equal(got, test.want) {
						t.Fatalf("reader %T with %d-byte destination: got %x, want %x", source, size, got, test.want)
					}
				}
			}
		})
	}
}

type utf8ErrorReader struct {
	data []byte
	err  error
}

func (r *utf8ErrorReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestUtf8DecoderReturnsDataBeforeError(t *testing.T) {
	wantErr := errors.New("source failed")
	decoder := NewUtf8Decoder(&utf8ErrorReader{data: []byte("A\u00e9\xc2"), err: wantErr})
	var buf [1]byte
	for _, want := range []byte{'A', 0xe9, 0xfd} {
		if n, err := decoder.Read(buf[:]); n != 1 || err != nil || buf[0] != want {
			t.Fatalf("Read = (%d, %v, %x), want (1, nil, %x)", n, err, buf[0], want)
		}
	}
	for range 2 {
		if n, err := decoder.Read(buf[:]); n != 0 || err != wantErr {
			t.Fatalf("Read after data = (%d, %v), want (0, source error)", n, err)
		}
	}
	if n, err := decoder.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil) after error = (%d, %v), want (0, nil)", n, err)
	}
}

func BenchmarkUtf16Count(b *testing.B) {
	for _, input := range []struct{ name, text string }{
		{"ascii", strings.Repeat("hello", 64)},
		{"unicode", strings.Repeat("A\u00e9\u4f60\U0001f642", 32)},
	} {
		b.Run(input.name, func(b *testing.B) {
			data := []byte(input.text)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if Utf16Count(data) == 0 {
					b.Fatal("empty count")
				}
			}
		})
	}
}

func BenchmarkUtf8Decoder(b *testing.B) {
	input := strings.Repeat("A\u00e9\u00ff", bufferSize)
	for _, size := range []int{1, bufferSize} {
		name := "full destination"
		if size == 1 {
			name = "one byte destination"
		}
		b.Run(name, func(b *testing.B) {
			buf := make([]byte, size)
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for b.Loop() {
				decoder := NewUtf8Decoder(strings.NewReader(input))
				for {
					_, err := decoder.Read(buf)
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
