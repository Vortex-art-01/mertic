package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const (
	Header = "HashSHA256"
	None   = "none"
)

func Sign(data []byte, key string) string {
	return hex.EncodeToString(sum(data, key))
}

func Valid(data []byte, key, want string) bool {
	got, err := hex.DecodeString(want)
	if err != nil {
		return false
	}

	return hmac.Equal(got, sum(data, key))
}

func sum(data []byte, key string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)

	return mac.Sum(nil)
}
