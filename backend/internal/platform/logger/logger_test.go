package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	if parseLevel("debug") != slog.LevelDebug || parseLevel("warn") != slog.LevelWarn {
		t.Fatal("unexpected level mapping")
	}
	if parseLevel("nope") != slog.LevelInfo {
		t.Fatal("unknown level should fall back to info")
	}
}

func TestNewFiltersDebugAtInfo(t *testing.T) {
	var buf bytes.Buffer
	log := newWith(&buf, "info")
	log.Debug("hidden", "token", "super-secret-token")
	log.Info("visible", "bot_id", 7)

	out := buf.String()
	if strings.Contains(out, "hidden") || strings.Contains(out, "super-secret-token") {
		t.Fatalf("debug record leaked: %s", out)
	}
	if !strings.Contains(out, `"msg":"visible"`) || !strings.Contains(out, `"bot_id":7`) {
		t.Fatalf("info record missing: %s", out)
	}
}
