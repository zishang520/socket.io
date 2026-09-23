package types

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestSliceSpliceOverlappingInput(t *testing.T) {
	// Replace retains the supplied array, so callers can insert values that
	// overlap the slice being changed, with or without room to grow in place.
	for size := range 7 {
		for _, capacity := range []int{size, size + 8} {
			for start := 0; start <= size; start++ {
				for deleteCount := -1; deleteCount <= size+1; deleteCount++ {
					for first := 0; first <= size; first++ {
						for last := first; last <= size; last++ {
							backing := make([]int, size, capacity)
							for i := range backing {
								backing[i] = i + 1
							}
							count := min(max(deleteCount, 0), size-start)
							wantRemoved := append([]int{}, backing[start:start+count]...)
							want := append([]int{}, backing[:start]...)
							want = append(want, backing[first:last]...)
							want = append(want, backing[start+count:]...)
							var values Slice[int]
							values.Replace(backing)
							removed, err := values.Splice(start, deleteCount, backing[first:last]...)
							if got := values.All(); err != nil || !slices.Equal(got, want) || !reflect.DeepEqual(removed, wantRemoved) {
								t.Fatalf("size=%d cap=%d start=%d delete=%d insert=[%d:%d]: got %v, removed %v, err %v; want %v, removed %v", size, capacity, start, deleteCount, first, last, got, removed, err, want, wantRemoved)
							}
							if len(removed) > 0 && values.Len() > 0 {
								removed[0] = -1
								if !slices.Equal(values.All(), want) {
									t.Fatal("removed values alias the retained slice")
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestSliceUnshiftRetainsInputOwnership(t *testing.T) {
	backing := make([]int, 3, 8)
	copy(backing, []int{1, 2, 3})
	var values Slice[int]
	values.Replace(backing)
	values.Unshift(backing[1:]...)
	if got := values.All(); !slices.Equal(got, []int{2, 3, 1, 2, 3}) {
		t.Fatalf("Unshift = %v", got)
	}
	if !slices.Equal(backing, []int{1, 2, 3}) {
		t.Fatalf("Unshift modified the previous backing array: %v", backing)
	}
}

func TestSliceRemovalClearsReferences(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*Slice[*int])
	}{
		{"Splice", func(values *Slice[*int]) { _, _ = values.Splice(1, 2) }},
		{"Remove", func(values *Slice[*int]) { values.Remove(func(value *int) bool { return *value == 2 }) }},
		{"RemoveAll", func(values *Slice[*int]) { values.RemoveAll(func(value *int) bool { return *value > 1 }) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			backing := []*int{new(1), new(2), new(3)}
			var values Slice[*int]
			values.Replace(backing)
			operation.run(&values)
			for _, value := range backing[values.Len():] {
				if value != nil {
					t.Fatal("removed reference retained in backing array")
				}
			}
		})
	}
}

func TestSliceRemovalVisitsEachValueOnce(t *testing.T) {
	values := NewSlice(1, 2, 3, 4, 5)
	var visited []int
	values.RemoveAll(func(value int) bool {
		visited = append(visited, value)
		return value%2 == 0
	})
	if !slices.Equal(visited, []int{1, 2, 3, 4, 5}) || !slices.Equal(values.All(), []int{1, 3, 5}) {
		t.Fatalf("RemoveAll visited %v, retained %v", visited, values.All())
	}
	visited = nil
	values.Remove(func(value int) bool {
		visited = append(visited, value)
		return value > 1
	})
	if !slices.Equal(visited, []int{1, 3}) || !slices.Equal(values.All(), []int{1, 5}) {
		t.Fatalf("Remove visited %v, retained %v", visited, values.All())
	}
}

func TestSliceRemovalPreservesEmptyEncoding(t *testing.T) {
	for _, initial := range [][]int{nil, {}} {
		var values Slice[int]
		values.Replace(initial)
		_, _ = values.Splice(0, 0)
		values.Remove(func(int) bool { t.Fatal("unexpected empty callback"); return true })
		values.RemoveAll(func(int) bool { t.Fatal("unexpected empty callback"); return true })
		for _, encode := range []struct {
			name string
			run  func(any) ([]byte, error)
		}{
			{"JSON", json.Marshal},
			{"MessagePack", msgpack.Marshal},
		} {
			want, err := encode.run(initial)
			if err != nil {
				t.Fatal(err)
			}
			got, err := encode.run(&values)
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("%s empty encoding: got %x, %v; want %x", encode.name, got, err, want)
			}
		}
	}
}

func TestAtomicInitialSwap(t *testing.T) {
	var number Atomic[int]
	if old := number.Swap(42); old != 0 || number.Load() != 42 {
		t.Fatalf("first Swap: old=%d current=%d", old, number.Load())
	}
	var pointer Atomic[*int]
	value := new(42)
	if old := pointer.Swap(value); old != nil || pointer.Load() != value {
		t.Fatalf("first pointer Swap: old=%v current=%v", old, pointer.Load())
	}
}

func TestAtomicInterfaceValues(t *testing.T) {
	var value Atomic[any]
	if value.Load() != nil {
		t.Fatal("zero-value interface must load nil")
	}
	var pointer *int
	value.Store(pointer)
	if got, ok := value.Load().(*int); !ok || got != nil {
		t.Fatalf("typed nil was not preserved: %v", value.Load())
	}
	replacement := new(42)
	if !value.CompareAndSwap(pointer, replacement) || value.Load() != replacement {
		t.Fatal("CAS must compare the typed nil value")
	}
	if old := value.Swap(pointer); old != replacement || value.Load() != pointer {
		t.Fatal("Swap must preserve interface values")
	}
	t.Run("consistent dynamic type", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("storing a different dynamic type must panic")
			}
		}()
		value.Store("different type")
	})
	t.Run("nil interface rejected", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("storing a nil interface must panic")
			}
		}()
		value.Store(nil)
	})
}

func BenchmarkSliceRemoveAllNoMatch(b *testing.B) {
	values := NewSlice(make([]int, 1024)...)
	b.ReportAllocs()
	for b.Loop() {
		values.RemoveAll(func(value int) bool { return value != 0 })
	}
}

func BenchmarkSliceSpliceReplace(b *testing.B) {
	values := NewSlice(make([]int, 128)...)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = values.Splice(63, 1, 42)
	}
}
