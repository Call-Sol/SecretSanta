package admin

import (
	"bytes"
	"regexp"
	"testing"
)

func TestInviteTokenAndDeeplink(t *testing.T) {
	raw, sum, err := newInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`).MatchString(raw) {
		t.Fatalf("token %q", raw)
	}
	if !bytes.Equal(sum, hashToken(raw)) {
		t.Fatal("hash mismatch")
	}
	if bytes.Contains(sum, []byte(raw)) {
		t.Fatal("hash contains the token")
	}
	link := Deeplink("secret_santa_bot", raw)
	want := "https://t.me/secret_santa_bot?start=join_" + raw
	if link != want {
		t.Fatalf("link %s", link)
	}
}
