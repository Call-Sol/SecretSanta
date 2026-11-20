package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Deeplink is the Telegram start link for a group invite token.
func Deeplink(botUsername, token string) string {
	return "https://t.me/" + botUsername + "?start=join_" + token
}

// PlayLink is the Telegram start link that confirms participation in one game.
func PlayLink(botUsername, token string) string {
	if botUsername == "" || token == "" {
		return ""
	}
	return "https://t.me/" + botUsername + "?start=play_" + token
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

func newInviteToken() (string, []byte, error) {
	raw, err := randomToken(16)
	if err != nil {
		return "", nil, err
	}
	return raw, hashToken(raw), nil
}
