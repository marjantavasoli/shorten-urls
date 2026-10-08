package shortener

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	MinCodeLength = 6
	MaxCodeLength = 8

	alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

var alphabetSize = big.NewInt(int64(len(alphabet)))

func RandomCode(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("invalid code length %d", n)
	}
	out := make([]byte, n)
	for i := range out {
		idx, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", fmt.Errorf("read random number: %w", err)
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}

func IsValidCode(s string) bool {
	if len(s) < MinCodeLength || len(s) > MaxCodeLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
