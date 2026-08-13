package types

import "testing"

var (
	benchmarkBoolSink bool
	benchmarkIntsSink []int
)

func BenchmarkMapKeys(b *testing.B) {
	var values Map[int, int]
	for i := range 128 {
		values.Store(i, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkIntsSink = values.Keys()
	}
}

func BenchmarkMapValues(b *testing.B) {
	var values Map[int, int]
	for i := range 128 {
		values.Store(i, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkIntsSink = values.Values()
	}
}

func BenchmarkSetKeys(b *testing.B) {
	values := NewSet[int]()
	for i := range 128 {
		values.Add(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkIntsSink = values.Keys()
	}
}

func BenchmarkSetHas(b *testing.B) {
	values := NewSet(1)
	b.ResetTimer()
	for b.Loop() {
		benchmarkBoolSink = values.Has(1)
	}
}

func BenchmarkSetAddExisting(b *testing.B) {
	values := NewSet(1)
	b.ResetTimer()
	for b.Loop() {
		benchmarkBoolSink = values.Add(1)
	}
}

func BenchmarkSetDeleteMissing(b *testing.B) {
	values := NewSet[int]()
	b.ResetTimer()
	for b.Loop() {
		benchmarkBoolSink = values.Delete(1)
	}
}

func BenchmarkSetAddDelete(b *testing.B) {
	values := NewSet[int]()
	b.ResetTimer()
	for b.Loop() {
		values.Add(1)
		values.Delete(1)
	}
}
