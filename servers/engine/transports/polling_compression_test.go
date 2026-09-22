package transports

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestPollingCompressionResponse(t *testing.T) {
	payload := []byte(strings.Repeat("polling compression payload\n", 128))
	for _, encoding := range []string{"gzip", "deflate", "br", "zstd"} {
		for _, binary := range []bool{false, true} {
			name := encoding + "/text"
			if binary {
				name = encoding + "/binary"
			}
			t.Run(name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil)
				request.Header.Set("Accept-Encoding", encoding)
				recorder := httptest.NewRecorder()
				ctx := types.NewHttpContext(recorder, request)
				t.Cleanup(ctx.Flush)
				transport := NewPolling(ctx)
				t.Cleanup(transport.OnClose)
				transport.SetHttpCompression(&types.HttpCompression{Threshold: 1})
				data := types.NewStringBuffer(payload)
				contentType := "text/plain; charset=UTF-8"
				if binary {
					data = types.NewBytesBuffer(payload)
					contentType = "application/octet-stream"
				}
				calls := 0
				transport.DoWrite(ctx, data, &packet.Options{Compress: new(true)}, func(err error) {
					calls++
					if err != nil {
						t.Errorf("compressed response failed: %v", err)
					}
				})
				if calls != 1 || recorder.Code != http.StatusOK {
					t.Fatalf("compressed response: callbacks=%d status=%d", calls, recorder.Code)
				}
				if got := recorder.Header().Get("Content-Encoding"); got != encoding {
					t.Fatalf("Content-Encoding = %q, want %q", got, encoding)
				}
				if got := recorder.Header().Get("Content-Type"); got != contentType {
					t.Errorf("Content-Type = %q, want %q", got, contentType)
				}
				if got := recorder.Header().Get("Content-Length"); got != strconv.Itoa(recorder.Body.Len()) {
					t.Errorf("Content-Length = %q, body has %d bytes", got, recorder.Body.Len())
				}
				if decoded := decodePollingCompression(t, encoding, recorder.Body.Bytes()); !bytes.Equal(decoded, payload) {
					t.Fatal("compressed response did not preserve the payload")
				}
			})
		}
	}
}

func decodePollingCompression(t *testing.T, encoding string, body []byte) []byte {
	t.Helper()
	input := bytes.NewReader(body)
	var reader io.ReadCloser
	switch encoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(input)
		if err != nil {
			t.Fatal(err)
		}
		reader = gzipReader
	case "deflate":
		reader = flate.NewReader(input)
	case "br":
		reader = io.NopCloser(brotli.NewReader(input))
	case "zstd":
		decoder, err := zstd.NewReader(input)
		if err != nil {
			t.Fatal(err)
		}
		reader = decoder.IOReadCloser()
	default:
		t.Fatalf("unexpected compression encoding %q", encoding)
	}
	defer func() { _ = reader.Close() }()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestPollingCompressionBypass(t *testing.T) {
	for _, scenario := range []string{"disabled", "packet_disabled", "below_threshold", "not_accepted"} {
		t.Run(scenario, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil)
			if scenario != "not_accepted" {
				request.Header.Set("Accept-Encoding", "gzip")
			}
			recorder := httptest.NewRecorder()
			ctx := types.NewHttpContext(recorder, request)
			t.Cleanup(ctx.Flush)
			transport := NewPolling(ctx)
			t.Cleanup(transport.OnClose)
			if scenario != "disabled" {
				threshold := 1
				if scenario == "below_threshold" {
					threshold = 1024
				}
				transport.SetHttpCompression(&types.HttpCompression{Threshold: threshold})
			}
			options := &packet.Options{}
			if scenario == "packet_disabled" {
				options.Compress = new(false)
			}
			transport.DoWrite(ctx, types.NewStringBufferString("4hello"), options, func(err error) {
				if err != nil {
					t.Errorf("uncompressed response failed: %v", err)
				}
			})
			if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Encoding") != "" || recorder.Body.String() != "4hello" {
				t.Fatalf("uncompressed response: status=%d encoding=%q body=%q", recorder.Code, recorder.Header().Get("Content-Encoding"), recorder.Body.String())
			}
		})
	}
}

type failedPollingCompressionInput struct {
	types.BufferInterface
	err error
}

func (b *failedPollingCompressionInput) WriteTo(io.Writer) (int64, error) {
	return 0, b.err
}

func TestPollingCompressionReadFailure(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate", "br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil)
			request.Header.Set("Accept-Encoding", encoding)
			recorder := httptest.NewRecorder()
			ctx := types.NewHttpContext(recorder, request)
			t.Cleanup(ctx.Flush)
			transport := NewPolling(ctx)
			t.Cleanup(transport.OnClose)
			transport.SetHttpCompression(&types.HttpCompression{Threshold: 1})
			readErr := errors.New("test compression input failure")
			data := &failedPollingCompressionInput{BufferInterface: types.NewStringBufferString("payload"), err: readErr}
			var callbackErr error
			transport.DoWrite(ctx, data, nil, func(err error) { callbackErr = err })
			if !errors.Is(callbackErr, readErr) || recorder.Code != http.StatusInternalServerError || recorder.Body.Len() != 0 {
				t.Fatalf("compression failure: callback=%v status=%d body=%q", callbackErr, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func BenchmarkPollingCompression(b *testing.B) {
	payload := strings.Repeat("engine.io polling payload\n", 256)
	for _, encoding := range []string{"gzip", "deflate", "br", "zstd"} {
		b.Run(encoding, func(b *testing.B) {
			transport := &polling{}
			data := types.NewStringBufferString(payload)
			if _, err := transport.compress(data, encoding); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for range b.N {
				data.Reset()
				_, _ = data.WriteString(payload)
				compressed, err := transport.compress(data, encoding)
				if err != nil || compressed.Len() == 0 {
					b.Fatalf("compression returned an empty result or failed: %v", err)
				}
			}
		})
	}
}
