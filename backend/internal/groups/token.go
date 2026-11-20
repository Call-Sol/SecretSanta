package groups

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func newInviteToken() (string, []byte, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("invite token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	hash := make([]byte, len(sum))
	copy(hash, sum[:])
	return raw, hash, nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}
