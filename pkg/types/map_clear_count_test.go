package types

import (
	"sync"
	"testing"
)

func TestMapCountAfterConcurrentClear(t *testing.T) {
	for _, test := range []struct {
		name      string
		deleted   bool
		operation func(*Map[string, int])
	}{
		{"delete", false, func(m *Map[string, int]) { m.Delete("key") }},
		{"compare delete", false, func(m *Map[string, int]) { m.CompareAndDelete("key", 1) }},
		{"store", false, func(m *Map[string, int]) { m.Store("new", 2) }},
		{"resurrect", true, func(m *Map[string, int]) { m.LoadOrStore("key", 2) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			for range 2000 {
				var values Map[string, int]
				values.Store("key", 1)
				values.Range(func(string, int) bool { return true })
				if test.deleted {
					values.Delete("key")
				}
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Go(func() { <-start; test.operation(&values) })
				wg.Go(func() { <-start; values.Clear() })
				close(start)
				wg.Wait()
				if count := len(values.Keys()); values.Len() != count {
					t.Fatalf("length = %d, want %d after concurrent Clear", values.Len(), count)
				}
				// A negative counter must not merely be hidden by clamping Len.
				values.Store("after", 3)
				if count := len(values.Keys()); values.Len() != count {
					t.Fatalf("length = %d, want %d after the next store", values.Len(), count)
				}
			}
		})
	}
}
