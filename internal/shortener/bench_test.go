package shortener

import "testing"

func BenchmarkRandomCode(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := RandomCode(MinCodeLength); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNormalizeURL(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := NormalizeURL("HTTPS://Example.COM:443/articles/2026/10/slug?utm_source=bench"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIsValidCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if !IsValidCode("aB3dE5g") {
			b.Fatal("invalid")
		}
	}
}
