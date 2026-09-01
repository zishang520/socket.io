package emitter

import "testing"

func TestEmitterPreservesExplicitEmptyNamespace(t *testing.T) {
	emitter := NewEmitter(nil, nil, "")
	if emitter.nsp != "" {
		t.Fatalf("namespace = %q, want an explicit empty namespace", emitter.nsp)
	}
}
