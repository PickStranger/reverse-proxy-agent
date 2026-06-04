package middleware

import (
	"crypto/rand"
	"encoding/hex"
)

func generateSessionToken() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(b), nil
}
