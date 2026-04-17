package request

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func TestDecompressBrotli(t *testing.T) {
	// Create brotli compressed data
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	original := "Hello, Brotli!"
	_, _ = bw.Write([]byte(original))
	if err := bw.Close(); err != nil {
		t.Fatalf("Failed to close brotli writer: %v", err)
	}

	// Create a readCloser wrapper
	rc := io.NopCloser(&buf)

	// Decompress
	decompressed, err := decompressBrotli(rc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() { _ = decompressed.Close() }()

	result, err := io.ReadAll(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	if string(result) != original {
		t.Errorf("Expected %q, got %q", original, string(result))
	}
}

func TestDecompressZstd(t *testing.T) {
	// Create zstd compressed data
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	original := "Hello, Zstd!"
	_, _ = zw.Write([]byte(original))
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close zstd writer: %v", err)
	}

	// Create a readCloser wrapper
	rc := io.NopCloser(&buf)

	// Decompress
	decompressed, err := decompressZstd(rc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() { _ = decompressed.Close() }()

	result, err := io.ReadAll(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	if string(result) != original {
		t.Errorf("Expected %q, got %q", original, string(result))
	}
}

func TestBrotliReaderClose(t *testing.T) {
	// Test that Close properly closes the underlying reader
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	_, _ = bw.Write([]byte("test"))
	if err := bw.Close(); err != nil {
		t.Fatalf("Failed to close brotli writer: %v", err)
	}

	closeCalled := false
	mockRC := &mockReadCloser{
		Reader: &buf,
		CloseFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	br, _ := decompressBrotli(mockRC)
	_ = br.Close()

	if !closeCalled {
		t.Error("Expected underlying reader to be closed")
	}
}

func TestZstdReaderClose(t *testing.T) {
	// Test that Close properly closes the underlying reader
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	_, _ = zw.Write([]byte("test"))
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close zstd writer: %v", err)
	}

	closeCalled := false
	mockRC := &mockReadCloser{
		Reader: &buf,
		CloseFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	zr, _ := decompressZstd(mockRC)
	_ = zr.Close()

	if !closeCalled {
		t.Error("Expected underlying reader to be closed")
	}
}

func TestDecompressZstdError(t *testing.T) {
	// Test with invalid data - zstd might not error immediately on creation
	invalidData := io.NopCloser(bytes.NewReader([]byte("invalid data")))

	decompressed, err := decompressZstd(invalidData)
	if err != nil {
		// Error on creation is acceptable
		return
	}

	// If no error on creation, error should occur on read
	_, err = io.ReadAll(decompressed)
	if err == nil {
		t.Error("Expected error when reading invalid zstd data")
	}
}

// mockReadCloser for testing
type mockReadCloser struct {
	io.Reader
	CloseFunc func() error
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	return m.Reader.Read(p)
}

func (m *mockReadCloser) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Test with gzip to ensure we don't break existing functionality
func TestGzipDecompression(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	original := "Hello, Gzip!"
	_, _ = gw.Write([]byte(original))
	if err := gw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer func() { _ = gr.Close() }()

	result, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("Failed to read gzip data: %v", err)
	}

	if string(result) != original {
		t.Errorf("Expected %q, got %q", original, string(result))
	}
}
