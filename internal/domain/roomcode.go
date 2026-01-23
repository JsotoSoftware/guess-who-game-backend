package domain

import (
	"crypto/rand"
)

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func NewRoomCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out), nil
}
