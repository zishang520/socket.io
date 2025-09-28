// Package slices provides utilities for safe slice operations with generics
package slices

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

// TestGet tests the Get function.
func TestGet(t *testing.T) {
	s := []int{10, 20, 30}
	var nilSlice []int

	testCases := []struct {
		name    string
		slice   []int
		idx     int
		wantVal int
		wantOk  bool
	}{
		{"first element", s, 0, 10, true},
		{"middle element", s, 1, 20, true},
		{"last element", s, 2, 30, true},
		{"index out of bounds (high)", s, 3, 0, false},
		{"index out of bounds (negative)", s, -1, 0, false},
		{"empty slice", []int{}, 0, 0, false},
		{"nil slice", nilSlice, 0, 0, false},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOk := Get(tt.slice, tt.idx)
			if gotVal != tt.wantVal || gotOk != tt.wantOk {
				t.Errorf("Get(%v, %d) = (%v, %v), want (%v, %v)", tt.slice, tt.idx, gotVal, gotOk, tt.wantVal, tt.wantOk)
			}
		})
	}
}

// TestGetAny tests the GetAny function.
func TestGetAny(t *testing.T) {
	s := []any{10, "hello", 30.5}

	t.Run("successful get int", func(t *testing.T) {
		gotVal, gotOk := GetAny[int](s, 0)
		if !gotOk || gotVal != 10 {
			t.Errorf(`GetAny[int](s, 0) = (%v, %v), want (10, true)`, gotVal, gotOk)
		}
	})

	t.Run("successful get string", func(t *testing.T) {
		gotVal, gotOk := GetAny[string](s, 1)
		if !gotOk || gotVal != "hello" {
			t.Errorf(`GetAny[string](s, 1) = (%v, %v), want ("hello", true)`, gotVal, gotOk)
		}
	})

	t.Run("type assertion failed", func(t *testing.T) {
		gotVal, gotOk := GetAny[int](s, 1) // index 1 is a string
		if gotOk || gotVal != 0 {
			t.Errorf(`GetAny[int](s, 1) = (%v, %v), want (0, false)`, gotVal, gotOk)
		}
	})

	t.Run("index out of bounds", func(t *testing.T) {
		gotVal, gotOk := GetAny[int](s, 10)
		if gotOk || gotVal != 0 {
			t.Errorf(`GetAny[int](s, 10) = (%v, %v), want (0, false)`, gotVal, gotOk)
		}
	})
}

// TestTryGet tests the TryGet function.
func TestTryGet(t *testing.T) {
	s := []string{"a", "b", "c"}
	var nilSlice []string

	testCases := []struct {
		name    string
		slice   []string
		idx     int
		wantVal string
	}{
		{"first element", s, 0, "a"},
		{"last element", s, 2, "c"},
		{"index out of bounds (high)", s, 3, ""},
		{"index out of bounds (negative)", s, -1, ""},
		{"empty slice", []string{}, 0, ""},
		{"nil slice", nilSlice, 0, ""},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			gotVal := TryGet(tt.slice, tt.idx)
			if gotVal != tt.wantVal {
				t.Errorf("TryGet(%v, %d) = %q, want %q", tt.slice, tt.idx, gotVal, tt.wantVal)
			}
		})
	}
}

// TestTryGetAny tests the TryGetAny function.
func TestTryGetAny(t *testing.T) {
	s := []any{10, "hello", 30.5}

	t.Run("successful get", func(t *testing.T) {
		got := TryGetAny[string](s, 1)
		if want := "hello"; got != want {
			t.Errorf(`TryGetAny[string](s, 1) = %q, want %q`, got, want)
		}
	})

	t.Run("type assertion failed", func(t *testing.T) {
		got := TryGetAny[int](s, 1) // index 1 is string
		if want := 0; got != want {
			t.Errorf(`TryGetAny[int](s, 1) = %d, want %d`, got, want)
		}
	})

	t.Run("index out of bounds", func(t *testing.T) {
		got := TryGetAny[float64](s, -1)
		if want := 0.0; got != want {
			t.Errorf(`TryGetAny[float64](s, -1) = %f, want %f`, got, want)
		}
	})
}

// TestGetWithDefault tests the GetWithDefault function.
func TestGetWithDefault(t *testing.T) {
	s := []int{10, 20, 30}
	defaultVal := 99

	testCases := []struct {
		name  string
		slice []int
		idx   int
		want  int
	}{
		{"get existing", s, 1, 20},
		{"get out of bounds", s, 5, defaultVal},
		{"get negative index", s, -1, defaultVal},
		{"get from empty", []int{}, 0, defaultVal},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWithDefault(tt.slice, tt.idx, defaultVal)
			if got != tt.want {
				t.Errorf("GetWithDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetPtr tests the GetPtr function.
func TestGetPtr(t *testing.T) {
	s := []int{10, 0, 30}

	t.Run("get existing element", func(t *testing.T) {
		ptr := GetPtr(s, 0)
		if ptr == nil {
			t.Fatal("GetPtr returned nil for a valid index")
		}
		if *ptr != 10 {
			t.Errorf("GetPtr returned pointer to %v, want 10", *ptr)
		}
	})

	t.Run("get existing zero value element", func(t *testing.T) {
		ptr := GetPtr(s, 1)
		if ptr == nil {
			t.Fatal("GetPtr returned nil for a valid index containing a zero value")
		}
		if *ptr != 0 {
			t.Errorf("GetPtr returned pointer to %v, want 0", *ptr)
		}
	})

	t.Run("get out of bounds", func(t *testing.T) {
		ptr := GetPtr(s, 10)
		if ptr != nil {
			t.Errorf("GetPtr should return nil for out of bounds index, got %v", *ptr)
		}
	})

	t.Run("get negative index", func(t *testing.T) {
		ptr := GetPtr(s, -1)
		if ptr != nil {
			t.Errorf("GetPtr should return nil for negative index, got %v", *ptr)
		}
	})
}

// TestSlice tests the Slice function.
func TestSlice(t *testing.T) {
	s := []int{0, 1, 2, 3, 4}

	testCases := []struct {
		name  string
		start int
		want  []int
	}{
		{"from beginning", 0, []int{0, 1, 2, 3, 4}},
		{"from middle", 2, []int{2, 3, 4}},
		{"from end", 5, []int{}},
		{"past end", 6, []int{}},
		{"negative one", -1, []int{4}},
		{"negative middle", -3, []int{2, 3, 4}},
		{"negative whole slice", -5, []int{0, 1, 2, 3, 4}},
		{"negative past beginning", -6, []int{0, 1, 2, 3, 4}},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := Slice(s, tt.start)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Slice(s, %d) = %v, want %v", tt.start, got, tt.want)
			}
		})
	}

	t.Run("empty slice", func(t *testing.T) {
		s := []int{}
		got := Slice(s, 0)
		if len(got) != 0 {
			t.Errorf("Slice on empty slice should be empty, got %v", got)
		}
		got = Slice(s, -1)
		if len(got) != 0 {
			t.Errorf("Slice on empty slice should be empty, got %v", got)
		}
	})
}

// TestFirstAndLast tests the First and Last functions.
func TestFirstAndLast(t *testing.T) {
	s := []string{"first", "middle", "last"}
	var empty []string
	var nilSlice []string

	// Test First
	val, ok := First(s)
	if !ok || val != "first" {
		t.Errorf(`First(s) = (%q, %v), want ("first", true)`, val, ok)
	}
	val, ok = First(empty)
	if ok || val != "" {
		t.Errorf(`First(empty) = (%q, %v), want ("", false)`, val, ok)
	}
	val, ok = First(nilSlice)
	if ok || val != "" {
		t.Errorf(`First(nil) = (%q, %v), want ("", false)`, val, ok)
	}

	// Test Last
	val, ok = Last(s)
	if !ok || val != "last" {
		t.Errorf(`Last(s) = (%q, %v), want ("last", true)`, val, ok)
	}
	val, ok = Last(empty)
	if ok || val != "" {
		t.Errorf(`Last(empty) = (%q, %v), want ("", false)`, val, ok)
	}
	val, ok = Last(nilSlice)
	if ok || val != "" {
		t.Errorf(`Last(nil) = (%q, %v), want ("", false)`, val, ok)
	}
}

// TestFilter tests the Filter function.
func TestFilter(t *testing.T) {
	s := []int{1, 2, 3, 4, 5, 6}
	isEven := func(n int) bool { return n%2 == 0 }

	got := Filter(s, isEven)
	want := []int{2, 4, 6}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Filter(isEven) = %v, want %v", got, want)
	}

	t.Run("filter none", func(t *testing.T) {
		isGreaterThan10 := func(n int) bool { return n > 10 }
		got := Filter(s, isGreaterThan10)
		if len(got) != 0 {
			t.Errorf("Filter(isGreaterThan10) should be empty, got %v", got)
		}
	})

	t.Run("filter all", func(t *testing.T) {
		isPositive := func(n int) bool { return n > 0 }
		got := Filter(s, isPositive)
		if !reflect.DeepEqual(got, s) {
			t.Errorf("Filter(isPositive) = %v, want %v", got, s)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := Filter([]int{}, isEven)
		if len(got) != 0 {
			t.Errorf("Filter on empty slice should be empty, got %v", got)
		}
	})
}

// TestMap tests the Map function.
func TestMap(t *testing.T) {
	s := []int{1, 2, 3}
	intToString := func(n int) string { return "v" + strconv.Itoa(n) }

	got := Map(s, intToString)
	want := []string{"v1", "v2", "v3"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Map(intToString) = %v, want %v", got, want)
	}

	t.Run("empty slice", func(t *testing.T) {
		got := Map([]int{}, intToString)
		if len(got) != 0 {
			t.Errorf("Map on empty slice should be empty, got %v", got)
		}
	})
}

// TestReduce tests the Reduce function.
func TestReduce(t *testing.T) {
	s := []int{1, 2, 3, 4}
	sumReducer := func(acc, val int) int { return acc + val }

	got := Reduce(s, 0, sumReducer)
	want := 10

	if got != want {
		t.Errorf("Reduce(sum) = %d, want %d", got, want)
	}

	t.Run("with initial value", func(t *testing.T) {
		got := Reduce(s, 100, sumReducer)
		want := 110
		if got != want {
			t.Errorf("Reduce(sum, 100) = %d, want %d", got, want)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := Reduce([]int{}, 5, sumReducer)
		want := 5
		if got != want {
			t.Errorf("Reduce on empty slice should return initial value, got %d want %d", got, want)
		}
	})
}

// TestIsEmpty tests the IsEmpty function.
func TestIsEmpty(t *testing.T) {
	var nilSlice []int
	emptySlice := []int{}
	nonEmptySlice := []int{1}

	if !IsEmpty(nilSlice) {
		t.Error("IsEmpty(nil) should be true")
	}
	if !IsEmpty(emptySlice) {
		t.Error("IsEmpty([]{}) should be true")
	}
	if IsEmpty(nonEmptySlice) {
		t.Error("IsEmpty({1}) should be false")
	}
}

// TestIsValidIndex tests the IsValidIndex function.
func TestIsValidIndex(t *testing.T) {
	s := []int{10, 20}

	if !IsValidIndex(s, 0) {
		t.Error("IsValidIndex(s, 0) should be true")
	}
	if !IsValidIndex(s, 1) {
		t.Error("IsValidIndex(s, 1) should be true")
	}
	if IsValidIndex(s, 2) {
		t.Error("IsValidIndex(s, 2) should be false")
	}
	if IsValidIndex(s, -1) {
		t.Error("IsValidIndex(s, -1) should be false")
	}
	if IsValidIndex([]int{}, 0) {
		t.Error("IsValidIndex on empty slice should be false")
	}
}

// --- Benchmarks ---

var (
	benchSliceInt   = makeBenchSlice[int](1000)
	benchSliceAny   = makeBenchSlice[any](1000)
	benchResultInt  int
	benchResultBool bool
)

func makeBenchSlice[E any](size int) []E {
	s := make([]E, size)
	for i := 0; i < size; i++ {
		var val any = i
		s[i] = val.(E)
	}
	return s
}

func BenchmarkGet_Hit(b *testing.B) {
	var r int
	var ok bool
	for i := 0; i < b.N; i++ {
		r, ok = Get(benchSliceInt, 500)
	}
	benchResultInt = r
	benchResultBool = ok
}

func BenchmarkGet_Miss(b *testing.B) {
	var r int
	var ok bool
	for i := 0; i < b.N; i++ {
		r, ok = Get(benchSliceInt, 2000)
	}
	benchResultInt = r
	benchResultBool = ok
}

func BenchmarkTryGet_Hit(b *testing.B) {
	var r int
	for i := 0; i < b.N; i++ {
		r = TryGet(benchSliceInt, 500)
	}
	benchResultInt = r
}

func BenchmarkTryGet_Miss(b *testing.B) {
	var r int
	for i := 0; i < b.N; i++ {
		r = TryGet(benchSliceInt, 2000)
	}
	benchResultInt = r
}

func BenchmarkGetAny_Hit(b *testing.B) {
	var r int
	var ok bool
	for i := 0; i < b.N; i++ {
		r, ok = GetAny[int](benchSliceAny, 500)
	}
	benchResultInt = r
	benchResultBool = ok
}

func BenchmarkGetAny_MissType(b *testing.B) {
	s := []any{"not an int"}
	var r int
	var ok bool
	for i := 0; i < b.N; i++ {
		r, ok = GetAny[int](s, 0)
	}
	benchResultInt = r
	benchResultBool = ok
}

func BenchmarkFilter(b *testing.B) {
	predicate := func(n int) bool { return n%2 == 0 }
	var result []int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = Filter(benchSliceInt, predicate)
	}
	_ = result
}

func BenchmarkMap(b *testing.B) {
	transform := func(n int) string { return fmt.Sprintf("val:%d", n) }
	var result []string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = Map(benchSliceInt, transform)
	}
	_ = result
}

func BenchmarkReduce(b *testing.B) {
	reducer := func(acc, n int) int { return acc + n }
	var result int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = Reduce(benchSliceInt, 0, reducer)
	}
	_ = result
}
