package utils

import (
	"errors"
	"testing"
)

func TestUrl(t *testing.T) {
	t.Run("valid http", func(t *testing.T) {
		p, err := Url("http://example.com", "/socket.io")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Hostname != "example.com" {
			t.Errorf("expected hostname example.com, got %s", p.Hostname)
		}
		if p.Port != "80" {
			t.Errorf("expected port 80, got %s", p.Port)
		}
		if p.Id != "http://example.com:80/socket.io" {
			t.Errorf("unexpected Id: %s", p.Id)
		}
	})

	t.Run("valid https with port", func(t *testing.T) {
		p, err := Url("https://example.com:8443/path", "/socket.io")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Port != "8443" {
			t.Errorf("expected port 8443, got %s", p.Port)
		}
	})

	t.Run("valid wss default port", func(t *testing.T) {
		p, err := Url("wss://example.com", "/socket.io")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Port != "443" {
			t.Errorf("expected port 443, got %s", p.Port)
		}
	})

	t.Run("empty URI", func(t *testing.T) {
		_, err := Url("", "/socket.io")
		if !errors.Is(err, ErrEmptyURI) {
			t.Errorf("expected ErrEmptyURI, got %v", err)
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, err := Url("ftp://example.com", "/socket.io")
		if !errors.Is(err, ErrUnsupportedScheme) {
			t.Errorf("expected ErrUnsupportedScheme, got %v", err)
		}
	})

	t.Run("missing scheme", func(t *testing.T) {
		_, err := Url("://example.com", "/socket.io")
		if err == nil {
			t.Error("expected error for missing scheme")
		}
	})

	t.Run("empty path gets default", func(t *testing.T) {
		p, err := Url("http://example.com", "/socket.io")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Path != "/" {
			t.Errorf("expected path /, got %s", p.Path)
		}
	})
}
