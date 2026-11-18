package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExitsNonZeroWithoutToken(t *testing.T) {
	if os.Getenv("BOT_TEST_CHILD") == "1" {
		main()
		return
	}

	t.Setenv("TELEGRAM_BOT_TOKEN", "super-secret-token")

	cmd := exec.Command(os.Args[0], "-test.run=^TestExitsNonZeroWithoutToken$")
	cmd.Env = childEnv()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, output: %s", out)
	}
	text := string(out)
	if !strings.Contains(text, "TELEGRAM_BOT_TOKEN is required") {
		t.Fatalf("exit error %v, output: %s", err, text)
	}
	if strings.Contains(text, "super-secret-token") {
		t.Fatalf("token leaked: %s", text)
	}
}

func childEnv() []string {
	drop := map[string]struct{}{
		"DATABASE_URL":            {},
		"TELEGRAM_BOT_TOKEN":      {},
		"TELEGRAM_BOT_USERNAME":   {},
		"TELEGRAM_MODE":           {},
		"TELEGRAM_WEBHOOK_SECRET": {},
		"HTTP_ADDR":               {},
		"LOG_LEVEL":               {},
		"OUTBOX_POLL_INTERVAL":    {},
		"OUTBOX_BATCH_SIZE":       {},
		"SESSION_TTL":             {},
		"BOT_TEST_CHILD":          {},
	}
	env := make([]string, 0, len(os.Environ())+3)
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if ok {
			if _, skip := drop[key]; skip {
				continue
			}
		}
		env = append(env, kv)
	}
	return append(env,
		"BOT_TEST_CHILD=1",
		"DATABASE_URL=postgres://localhost/secretsanta?sslmode=disable",
		"TELEGRAM_BOT_USERNAME=secret_santa_bot",
	)
}
