package hash

import "testing"

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
