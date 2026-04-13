package types

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestBuffer_Seek(t *testing.T) {
	buf := NewBufferString("Hello World")

	// SeekStart
	pos, err := buf.Seek(5, io.SeekStart)
	if err != nil || pos != 5 {
		t.Errorf("Seek(5, SeekStart) = %d, %v; want 5, nil", pos, err)
	}
	b, _ := buf.ReadByte()
	if b != ' ' {
		t.Errorf("After seek to 5, ReadByte() = %c, want ' '", b)
	}

	// SeekCurrent
	pos, err = buf.Seek(2, io.SeekCurrent)
	if err != nil || pos != 8 {
		t.Errorf("Seek(2, SeekCurrent) = %d, %v; want 8, nil", pos, err)
	}

	// SeekEnd
	pos, err = buf.Seek(-3, io.SeekEnd)
	if err != nil || pos != 8 {
		t.Errorf("Seek(-3, SeekEnd) = %d, %v; want 8, nil", pos, err)
	}

	// Invalid whence
	_, err = buf.Seek(0, 99)
	if err == nil {
		t.Error("Seek with invalid whence should return error")
	}

	// Negative position
	_, err = buf.Seek(-1, io.SeekStart)
	if err == nil {
		t.Error("Seek to negative position should return error")
	}

	// Past end
	_, err = buf.Seek(100, io.SeekStart)
	if err == nil {
		t.Error("Seek past end should return error")
	}
}

func TestBuffer_Size(t *testing.T) {
	buf := NewBufferString("Hello")
	if got := buf.Size(); got != 5 {
		t.Errorf("Size() = %d, want 5", got)
	}
	// Read some bytes - Size should reflect total buf, not remaining
	_, _ = buf.ReadByte()
	if got := buf.Size(); got != 5 {
		t.Errorf("Size() after read = %d, want 5", got)
	}
}

func TestBuffer_Clone(t *testing.T) {
	buf := NewBufferString("Hello World")
	_, _ = buf.ReadByte() // advance offset
	clone := buf.Clone()
	if clone.String() != buf.String() {
		t.Errorf("Clone String() = %q, want %q", clone.String(), buf.String())
	}
	// Modify clone shouldn't affect original
	clone.Reset()
	if buf.Len() == 0 {
		t.Error("Modifying clone should not affect original")
	}
}

func TestBuffer_CloneNil(t *testing.T) {
	var buf *Buffer
	clone := buf.Clone()
	if clone != nil {
		t.Error("Clone of nil buffer should return nil")
	}
}

func TestIndexByte(t *testing.T) {
	if got := IndexByte([]byte("Hello"), 'l'); got != 2 {
		t.Errorf("IndexByte('Hello', 'l') = %d, want 2", got)
	}
	if got := IndexByte([]byte("Hello"), 'z'); got != -1 {
		t.Errorf("IndexByte('Hello', 'z') = %d, want -1", got)
	}
	if got := IndexByte([]byte{}, 'a'); got != -1 {
		t.Errorf("IndexByte(empty, 'a') = %d, want -1", got)
	}
}

func TestBytesBuffer(t *testing.T) {
	t.Run("NewBytesBuffer", func(t *testing.T) {
		buf := NewBytesBuffer([]byte("hello"))
		if buf.String() != "hello" {
			t.Errorf("String() = %q, want hello", buf.String())
		}
	})

	t.Run("NewBytesBufferString", func(t *testing.T) {
		buf := NewBytesBufferString("world")
		if buf.String() != "world" {
			t.Errorf("String() = %q, want world", buf.String())
		}
	})

	t.Run("NewBytesBufferReader", func(t *testing.T) {
		buf, err := NewBytesBufferReader(strings.NewReader("from reader"))
		if err != nil {
			t.Fatalf("NewBytesBufferReader error: %v", err)
		}
		if buf.String() != "from reader" {
			t.Errorf("String() = %q, want 'from reader'", buf.String())
		}
	})

	t.Run("Clone", func(t *testing.T) {
		buf := NewBytesBuffer([]byte("test"))
		clone := buf.Clone()
		if clone == nil {
			t.Fatal("Clone returned nil")
		}
		if clone.String() != "test" {
			t.Errorf("Clone String() = %q, want test", clone.String())
		}
	})

	t.Run("Clone nil", func(t *testing.T) {
		var buf *BytesBuffer
		clone := buf.Clone()
		if clone != nil {
			t.Error("Clone of nil should return nil")
		}
	})

	t.Run("Clone nil buffer", func(t *testing.T) {
		buf := &BytesBuffer{}
		clone := buf.Clone()
		if clone != nil {
			t.Error("Clone of empty BytesBuffer should return nil")
		}
	})

	t.Run("GoString", func(t *testing.T) {
		buf := NewBytesBuffer([]byte{1, 2, 3})
		gs := buf.(interface{ GoString() string }).GoString()
		if gs == "" {
			t.Error("GoString() returned empty string")
		}
	})

	t.Run("GoString nil", func(t *testing.T) {
		var buf *BytesBuffer
		if buf.GoString() != "<nil>" {
			t.Errorf("GoString of nil = %q, want <nil>", buf.GoString())
		}
	})
}

func TestStringBuffer(t *testing.T) {
	t.Run("NewStringBuffer", func(t *testing.T) {
		buf := NewStringBuffer([]byte("hello"))
		if buf.String() != "hello" {
			t.Errorf("String() = %q, want hello", buf.String())
		}
	})

	t.Run("NewStringBufferString", func(t *testing.T) {
		buf := NewStringBufferString("world")
		if buf.String() != "world" {
			t.Errorf("String() = %q, want world", buf.String())
		}
	})

	t.Run("NewStringBufferReader", func(t *testing.T) {
		buf, err := NewStringBufferReader(strings.NewReader("from reader"))
		if err != nil {
			t.Fatalf("NewStringBufferReader error: %v", err)
		}
		if buf.String() != "from reader" {
			t.Errorf("String() = %q, want 'from reader'", buf.String())
		}
	})

	t.Run("Clone", func(t *testing.T) {
		buf := NewStringBuffer([]byte("test"))
		clone := buf.Clone()
		if clone == nil {
			t.Fatal("Clone returned nil")
		}
		if clone.String() != "test" {
			t.Errorf("Clone String() = %q, want test", clone.String())
		}
	})

	t.Run("Clone nil", func(t *testing.T) {
		var buf *StringBuffer
		clone := buf.Clone()
		if clone != nil {
			t.Error("Clone of nil should return nil")
		}
	})

	t.Run("GoString", func(t *testing.T) {
		buf := NewStringBufferString("hello")
		gs := buf.(interface{ GoString() string }).GoString()
		if gs != "hello" {
			t.Errorf("GoString() = %q, want hello", gs)
		}
	})

	t.Run("GoString nil", func(t *testing.T) {
		var buf *StringBuffer
		if buf.GoString() != "<nil>" {
			t.Errorf("GoString of nil = %q, want <nil>", buf.GoString())
		}
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		buf := NewStringBufferString("hello")
		data, err := json.Marshal(buf)
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}
		if string(data) != `"hello"` {
			t.Errorf("MarshalJSON = %s, want \"hello\"", data)
		}
	})

	t.Run("MarshalJSON nil", func(t *testing.T) {
		var buf *StringBuffer
		data, err := buf.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON nil error: %v", err)
		}
		if string(data) != `""` {
			t.Errorf("MarshalJSON nil = %s, want \"\"", data)
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		sb := &StringBuffer{NewBufferString("")}
		if err := json.Unmarshal([]byte(`"world"`), sb); err != nil {
			t.Fatalf("UnmarshalJSON error: %v", err)
		}
		if sb.String() != "world" {
			t.Errorf("After UnmarshalJSON, String() = %q, want world", sb.String())
		}
	})

	t.Run("UnmarshalJSON nil receiver", func(t *testing.T) {
		var sb *StringBuffer
		err := sb.UnmarshalJSON([]byte(`"test"`))
		if err != nil {
			t.Errorf("UnmarshalJSON on nil should return nil, got %v", err)
		}
	})

	t.Run("UnmarshalJSON invalid", func(t *testing.T) {
		sb := &StringBuffer{NewBufferString("")}
		err := sb.UnmarshalJSON([]byte(`not json`))
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

// Set extended tests
func TestSet_Keys(t *testing.T) {
	s := NewSet(1, 2, 3)
	keys := s.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() len = %d, want 3", len(keys))
	}
	// Verify all keys present
	m := make(map[int]bool)
	for _, k := range keys {
		m[k] = true
	}
	for _, expected := range []int{1, 2, 3} {
		if !m[expected] {
			t.Errorf("Keys() missing %d", expected)
		}
	}
}

func TestSet_MarshalJSON(t *testing.T) {
	s := NewSet("a", "b")
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal result error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("MarshalJSON produced %d elements, want 2", len(result))
	}
}

func TestSet_UnmarshalJSON(t *testing.T) {
	s := &Set[string]{}
	if err := json.Unmarshal([]byte(`["x","y","z"]`), s); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if s.Len() != 3 {
		t.Errorf("After UnmarshalJSON, Len() = %d, want 3", s.Len())
	}
	if !s.Has("x") || !s.Has("y") || !s.Has("z") {
		t.Error("UnmarshalJSON missing expected keys")
	}
}

func TestSet_UnmarshalJSONInvalid(t *testing.T) {
	s := &Set[string]{}
	err := json.Unmarshal([]byte(`not json`), s)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestSet_MarshalMsgpack(t *testing.T) {
	s := NewSet(10, 20, 30)
	data, err := msgpack.Marshal(s)
	if err != nil {
		t.Fatalf("MarshalMsgpack error: %v", err)
	}
	s2 := &Set[int]{}
	if err := msgpack.Unmarshal(data, s2); err != nil {
		t.Fatalf("UnmarshalMsgpack error: %v", err)
	}
	if s2.Len() != 3 {
		t.Errorf("After UnmarshalMsgpack, Len() = %d, want 3", s2.Len())
	}
}

func TestSet_UnmarshalMsgpackInvalid(t *testing.T) {
	s := &Set[string]{}
	err := msgpack.Unmarshal([]byte{0xFF, 0xFF}, s)
	if err == nil {
		t.Error("Expected error for invalid msgpack data")
	}
}

func TestSet_AddEmpty(t *testing.T) {
	s := NewSet[int]()
	if s.Add() {
		t.Error("Add() with no args should return false")
	}
}

func TestSet_DeleteEmpty(t *testing.T) {
	s := NewSet(1, 2)
	if s.Delete() {
		t.Error("Delete() with no args should return false")
	}
}

// Slice extended tests
func TestSlice_Filter(t *testing.T) {
	s := NewSlice(1, 2, 3, 4, 5, 6)
	evens := s.Filter(func(v int) bool { return v%2 == 0 })
	if len(evens) != 3 {
		t.Errorf("Filter evens: len = %d, want 3", len(evens))
	}
	for _, v := range evens {
		if v%2 != 0 {
			t.Errorf("Filter returned odd number: %d", v)
		}
	}
}

func TestSlice_FindIndex(t *testing.T) {
	s := NewSlice("a", "b", "c", "d")
	idx := s.FindIndex(func(v string) bool { return v == "c" })
	if idx != 2 {
		t.Errorf("FindIndex('c') = %d, want 2", idx)
	}
	idx = s.FindIndex(func(v string) bool { return v == "z" })
	if idx != -1 {
		t.Errorf("FindIndex('z') = %d, want -1", idx)
	}
}

func TestSlice_RemoveAll(t *testing.T) {
	s := NewSlice(1, 2, 3, 2, 4, 2)
	s.RemoveAll(func(v int) bool { return v == 2 })
	if s.Len() != 3 {
		t.Errorf("After RemoveAll(2), Len() = %d, want 3", s.Len())
	}
	all := s.All()
	for _, v := range all {
		if v == 2 {
			t.Error("RemoveAll did not remove all 2s")
		}
	}
}

func TestSlice_DoRead(t *testing.T) {
	s := NewSlice(10, 20, 30)
	var sum int
	s.DoRead(func(elements []int) {
		for _, v := range elements {
			sum += v
		}
	})
	if sum != 60 {
		t.Errorf("DoRead sum = %d, want 60", sum)
	}
}

func TestSlice_DoWrite(t *testing.T) {
	s := NewSlice(1, 2, 3)
	s.DoWrite(func(elements []int) []int {
		result := make([]int, len(elements))
		for i, v := range elements {
			result[i] = v * 2
		}
		return result
	})
	expected := []int{2, 4, 6}
	all := s.All()
	for i, v := range all {
		if v != expected[i] {
			t.Errorf("DoWrite: element[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestSlice_Replace(t *testing.T) {
	s := NewSlice(1, 2, 3)
	s.Replace([]int{10, 20})
	if s.Len() != 2 {
		t.Errorf("After Replace, Len() = %d, want 2", s.Len())
	}
	v, _ := s.Get(0)
	if v != 10 {
		t.Errorf("After Replace, Get(0) = %d, want 10", v)
	}
}

func TestSlice_RangeReverse(t *testing.T) {
	s := NewSlice(1, 2, 3, 4)
	var result []int
	s.Range(func(v, _ int) bool {
		result = append(result, v)
		return true
	}, true)
	expected := []int{4, 3, 2, 1}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("Range reverse: element[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestSlice_RangeAndSplice(t *testing.T) {
	s := NewSlice(1, 2, 3, 4, 5)
	removed, err := s.RangeAndSplice(func(v, i int) (bool, int, int, []int) {
		if v == 3 {
			return true, i, 1, []int{30, 31}
		}
		return false, 0, 0, nil
	})
	if err != nil {
		t.Fatalf("RangeAndSplice error: %v", err)
	}
	if len(removed) != 1 || removed[0] != 3 {
		t.Errorf("RangeAndSplice removed = %v, want [3]", removed)
	}
	if s.Len() != 6 {
		t.Errorf("After RangeAndSplice, Len() = %d, want 6", s.Len())
	}
}

func TestSlice_RangeAndSpliceReverse(t *testing.T) {
	s := NewSlice(1, 2, 3, 4, 5)
	removed, err := s.RangeAndSplice(func(v, i int) (bool, int, int, []int) {
		if v == 4 {
			return true, i, 1, nil
		}
		return false, 0, 0, nil
	}, true)
	if err != nil {
		t.Fatalf("RangeAndSplice reverse error: %v", err)
	}
	if len(removed) != 1 || removed[0] != 4 {
		t.Errorf("RangeAndSplice reverse removed = %v, want [4]", removed)
	}
}

func TestSlice_RangeAndSpliceNoMatch(t *testing.T) {
	s := NewSlice(1, 2, 3)
	removed, err := s.RangeAndSplice(func(v, i int) (bool, int, int, []int) {
		return false, 0, 0, nil
	})
	if err != nil || removed != nil {
		t.Errorf("RangeAndSplice no match: removed=%v, err=%v", removed, err)
	}
}

func TestSlice_ErrorPaths(t *testing.T) {
	t.Run("Pop empty", func(t *testing.T) {
		s := NewSlice[int]()
		_, err := s.Pop()
		if err != ErrSliceEmpty {
			t.Errorf("Pop empty: err = %v, want ErrSliceEmpty", err)
		}
	})

	t.Run("Shift empty", func(t *testing.T) {
		s := NewSlice[int]()
		_, err := s.Shift()
		if err != ErrSliceEmpty {
			t.Errorf("Shift empty: err = %v, want ErrSliceEmpty", err)
		}
	})

	t.Run("Get out of bounds", func(t *testing.T) {
		s := NewSlice(1, 2, 3)
		_, err := s.Get(-1)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Get(-1): err = %v, want ErrIndexOutOfBounds", err)
		}
		_, err = s.Get(3)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Get(3): err = %v, want ErrIndexOutOfBounds", err)
		}
	})

	t.Run("Set out of bounds", func(t *testing.T) {
		s := NewSlice(1, 2, 3)
		err := s.Set(-1, 0)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Set(-1): err = %v, want ErrIndexOutOfBounds", err)
		}
		err = s.Set(3, 0)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Set(3): err = %v, want ErrIndexOutOfBounds", err)
		}
	})

	t.Run("Slice invalid range", func(t *testing.T) {
		s := NewSlice(1, 2, 3)
		_, err := s.Slice(-1, 2)
		if err != ErrInvalidSliceRange {
			t.Errorf("Slice(-1,2): err = %v, want ErrInvalidSliceRange", err)
		}
		_, err = s.Slice(0, 4)
		if err != ErrInvalidSliceRange {
			t.Errorf("Slice(0,4): err = %v, want ErrInvalidSliceRange", err)
		}
		_, err = s.Slice(2, 1)
		if err != ErrInvalidSliceRange {
			t.Errorf("Slice(2,1): err = %v, want ErrInvalidSliceRange", err)
		}
	})

	t.Run("Splice out of bounds", func(t *testing.T) {
		s := NewSlice(1, 2, 3)
		_, err := s.Splice(-1, 1)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Splice(-1,1): err = %v, want ErrIndexOutOfBounds", err)
		}
		_, err = s.Splice(4, 1)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Splice(4,1): err = %v, want ErrIndexOutOfBounds", err)
		}
	})
}

func TestSlice_SpliceShrink(t *testing.T) {
	s := NewSlice(1, 2, 3, 4, 5)
	// Remove 3 elements, insert 1 (shrink)
	removed, err := s.Splice(1, 3, 99)
	if err != nil {
		t.Fatalf("Splice error: %v", err)
	}
	if len(removed) != 3 {
		t.Errorf("Splice removed %d elements, want 3", len(removed))
	}
	if s.Len() != 3 {
		t.Errorf("After Splice shrink, Len() = %d, want 3", s.Len())
	}
	all := s.All()
	expected := []int{1, 99, 5}
	for i, v := range all {
		if v != expected[i] {
			t.Errorf("Splice shrink: element[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestSlice_SpliceNegativeDeleteCount(t *testing.T) {
	s := NewSlice(1, 2, 3)
	removed, err := s.Splice(1, -5, 99)
	if err != nil {
		t.Fatalf("Splice error: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("Splice with negative deleteCount removed %d, want 0", len(removed))
	}
	// 99 inserted at index 1
	if s.Len() != 4 {
		t.Errorf("After Splice, Len() = %d, want 4", s.Len())
	}
}

// Map extended tests
func TestMap_LenKeysValues(t *testing.T) {
	m := &Map[string, int]{}
	m.Store("a", 1)
	m.Store("b", 2)
	m.Store("c", 3)

	if got := m.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}

	keys := m.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() len = %d, want 3", len(keys))
	}

	values := m.Values()
	if len(values) != 3 {
		t.Errorf("Values() len = %d, want 3", len(values))
	}

	// Verify sum of values
	sum := 0
	for _, v := range values {
		sum += v
	}
	if sum != 6 {
		t.Errorf("Sum of values = %d, want 6", sum)
	}
}

func TestMap_LenAfterDelete(t *testing.T) {
	m := &Map[string, int]{}
	m.Store("a", 1)
	m.Store("b", 2)
	m.Delete("a")
	if got := m.Len(); got != 1 {
		t.Errorf("Len() after delete = %d, want 1", got)
	}
}
