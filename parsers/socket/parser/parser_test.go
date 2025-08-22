package parser

import (
	"testing"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	if p == nil {
		t.Fatalf("Expected non-nil parser, got nil")
	}

	encoder := p.NewEncoder()
	if encoder == nil {
		t.Fatalf("Expected non-nil encoder, got nil")
	}

	decoder := p.NewDecoder()
	if decoder == nil {
		t.Fatalf("Expected non-nil decoder, got nil")
	}
}
