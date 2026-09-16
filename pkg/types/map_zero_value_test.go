package types

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestMapZeroSizedValues(t *testing.T) {
	var values Map[string, struct{}]
	values.Store("first", struct{}{})
	if _, ok := values.Load("first"); !ok {
		t.Fatal("stored zero-sized value is missing")
	}
	if _, loaded := values.LoadOrStore("first", struct{}{}); !loaded {
		t.Fatal("existing zero-sized value was treated as deleted")
	}
	values.Range(func(string, struct{}) bool { return true })
	if !values.CompareAndDelete("first", struct{}{}) {
		t.Fatal("zero-sized value could not be deleted")
	}
	values.Store("second", struct{}{})
	if _, loaded := values.LoadOrStore("first", struct{}{}); loaded {
		t.Fatal("deleted zero-sized value was not inserted again")
	}
	if values.Len() != 2 || len(values.Keys()) != 2 {
		t.Fatalf("length/keys = %d/%v, want two entries", values.Len(), values.Keys())
	}
	var concurrent Map[string, struct{}]
	var inserted atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, loaded := concurrent.LoadOrStore("key", struct{}{}); !loaded {
				inserted.Add(1)
			}
		})
	}
	wg.Wait()
	if inserted.Load() != 1 || concurrent.Len() != 1 {
		t.Fatalf("insertions/length = %d/%d, want 1/1", inserted.Load(), concurrent.Len())
	}
}
