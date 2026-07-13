package utils

import "testing"

var benchmarkStringSink string

func BenchmarkIsValidSID(b *testing.B) {
	const sid = "yH8rZp1uWq3xA7cN9mK2vB4d"
	b.ReportAllocs()
	for b.Loop() {
		if !IsValidSid(sid) {
			b.Fatal("valid SID rejected")
		}
	}
}

func BenchmarkYeastEncode(b *testing.B) {
	yeast := NewYeast()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkStringSink = yeast.Encode(1_752_345_678_901)
	}
}

func BenchmarkYeastDecode(b *testing.B) {
	yeast := NewYeast()
	encoded := yeast.Encode(1_752_345_678_901)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := yeast.Decode(encoded); err != nil {
			b.Fatal(err)
		}
	}
}
