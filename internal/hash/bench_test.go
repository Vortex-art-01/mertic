package hash

import (
	"strings"
	"testing"
)

// benchPayload по размеру как пакет метрик от агента: около 2,5 КБ.
var benchPayload = []byte("[" + strings.Repeat(`{"id":"HeapAlloc","type":"gauge","value":123456.5},`, 49) + "{}]")

func BenchmarkSign(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Sign(benchPayload, key)
	}
}

func BenchmarkValid(b *testing.B) {
	sign := Sign(benchPayload, key)

	b.ReportAllocs()
	for b.Loop() {
		if !Valid(benchPayload, key, sign) {
			b.Fatal("signature must be valid")
		}
	}
}

// Signer — то, чем пользуется сервер: подпись на каждый ответ и проверка на запрос.
func BenchmarkSigner(b *testing.B) {
	signer := NewSigner(key)
	sign := signer.Sign(benchPayload)

	b.Run("Sign", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = signer.Sign(benchPayload)
		}
	})

	b.Run("Valid", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if !signer.Valid(benchPayload, sign) {
				b.Fatal("signature must be valid")
			}
		}
	})
}
