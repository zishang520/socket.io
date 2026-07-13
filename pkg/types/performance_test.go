package types

import "testing"

var benchmarkIntsSink []int

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
