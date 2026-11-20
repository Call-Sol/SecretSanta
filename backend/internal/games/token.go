package games

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func newInviteToken() (string, []byte, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("game invite: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}
