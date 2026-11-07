package hash

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

const (
	key  = "secret"
	data = `[{"id":"Alloc","type":"gauge","value":123.45}]`
)

func TestSignIsStableAndKeyDependent(t *testing.T) {
	got := Sign([]byte(data), key)

	if want := Sign([]byte(data), key); got != want {
		t.Errorf("Sign is not stable: %q != %q", got, want)
	}
	if other := Sign([]byte(data), "another"); got == other {
		t.Error("signatures with different keys must differ")
	}
	if other := Sign([]byte(data+" "), key); got == other {
		t.Error("signatures of different data must differ")
	}
	if len(got) != 64 {
		t.Errorf("signature length = %d, want 64 hex characters of SHA-256", len(got))
	}
}

// Известный ответ HMAC-SHA256 фиксирует формат подписи независимо от реализации:
// её должны понимать и агент, и любой другой клиент.
func TestSignMatchesKnownVector(t *testing.T) {
	const want = "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"

	message := []byte("The quick brown fox jumps over the lazy dog")

	if got := Sign(message, "key"); got != want {
		t.Errorf("Sign() = %s, want %s", got, want)
	}
	if got := NewSigner("key").Sign(message); got != want {
		t.Errorf("Signer.Sign() = %s, want %s", got, want)
	}
}

func TestSignerValid(t *testing.T) {
	signer := NewSigner(key)
	sign := signer.Sign([]byte(data))

	tests := []struct {
		name string
		data string
		want string
		ok   bool
	}{
		{name: "own signature", data: data, want: sign, ok: true},
		{name: "upper case hex", data: data, want: strings.ToUpper(sign), ok: true},
		{name: "signed with another key", data: data, want: Sign([]byte(data), "another")},
		{name: "changed body", data: data + " ", want: sign},
		{name: "empty signature", data: data},
		{name: "too short", data: data, want: sign[:len(sign)-2]},
		{name: "too long", data: data, want: sign + "00"},
		{name: "odd length", data: data, want: sign[:len(sign)-1]},
		{name: "right length, not hex", data: data, want: strings.Repeat("z", len(sign))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := signer.Valid([]byte(tt.data), tt.want); got != tt.ok {
				t.Errorf("Valid() = %v, want %v", got, tt.ok)
			}
		})
	}
}

// Сигнатуры считаются повторно одними и теми же вычислителями из пула: прошлое
// сообщение не должно влиять на следующее, а одновременные вызовы — друг на друга.
func TestSignerConcurrentUseAndReuse(t *testing.T) {
	signer := NewSigner(key)

	var running sync.WaitGroup

	for g := range 16 {
		running.Go(func() {
			for i := range 200 {
				message := fmt.Appendf(nil, "горутина %d, сообщение %d", g, i)

				got := signer.Sign(message)
				if want := Sign(message, key); got != want {
					t.Errorf("Sign(%s) = %s, want %s", message, got, want)
				}
				if !signer.Valid(message, got) {
					t.Errorf("Valid(%s) = false for its own signature", message)
				}
			}
		})
	}

	running.Wait()
}

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		data string
		key  string
		want string
		ok   bool
	}{
		{name: "own signature", data: data, key: key, want: Sign([]byte(data), key), ok: true},
		{name: "empty body", data: "", key: key, want: Sign(nil, key), ok: true},
		{name: "other key", data: data, key: key, want: Sign([]byte(data), "another")},
		{name: "changed body", data: data + " ", key: key, want: Sign([]byte(data), key)},
		{name: "not a hex string", data: data, key: key, want: "не подпись"},
		{name: "empty signature", data: data, key: key, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid([]byte(tt.data), tt.key, tt.want); got != tt.ok {
				t.Errorf("Valid() = %v, want %v", got, tt.ok)
			}
		})
	}
}
