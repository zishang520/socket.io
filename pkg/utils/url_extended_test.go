package utils

import (
	"testing"
)

func TestUrlWsScheme(t *testing.T) {
	p, err := Url("ws://example.com", "/socket.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Hostname != "example.com" {
		t.Errorf("expected hostname example.com, got %s", p.Hostname)
	}
	if p.Port != "80" {
		t.Errorf("expected port 80, got %s", p.Port)
	}
	if p.Scheme != "ws" {
		t.Errorf("expected scheme ws, got %s", p.Scheme)
	}
}

func TestUrlIPv6(t *testing.T) {
	p, err := Url("http://[::1]:8080", "/socket.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Hostname != "::1" {
		t.Errorf("expected hostname ::1, got %s", p.Hostname)
	}
	if p.Port != "8080" {
		t.Errorf("expected port 8080, got %s", p.Port)
	}
}

func TestUrlWithQueryFragment(t *testing.T) {
	p, err := Url("https://example.com/path?key=value#frag", "/socket.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Port != "443" {
		t.Errorf("expected port 443, got %s", p.Port)
	}
	if p.RawQuery != "key=value" {
		t.Errorf("expected query key=value, got %s", p.RawQuery)
	}
}

func TestUrlWithExplicitPath(t *testing.T) {
	p, err := Url("http://example.com/existing", "/socket.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Path != "/existing" {
		t.Errorf("expected path /existing, got %s", p.Path)
	}
}
