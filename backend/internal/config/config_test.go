package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_BOT_USERNAME", "secret_santa_bot")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadRequiresBotToken(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")
	t.Setenv("TELEGRAM_BOT_USERNAME", "secret_santa_bot")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN is required") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("database url leaked: %v", err)
	}
}

func TestLoadRequiresBotUsername(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")
	t.Setenv("TELEGRAM_BOT_TOKEN", "super-secret-token")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_USERNAME is required") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{name: "mode", key: "TELEGRAM_MODE", val: "socket", want: "TELEGRAM_MODE"},
		{name: "level", key: "LOG_LEVEL", val: "verbose", want: "LOG_LEVEL"},
		{name: "interval", key: "OUTBOX_POLL_INTERVAL", val: "nope", want: "OUTBOX_POLL_INTERVAL"},
		{name: "batch", key: "OUTBOX_BATCH_SIZE", val: "0", want: "OUTBOX_BATCH_SIZE"},
		{name: "ttl", key: "SESSION_TTL", val: "0s", want: "SESSION_TTL"},
		{name: "username", key: "TELEGRAM_BOT_USERNAME", val: "bad name", want: "TELEGRAM_BOT_USERNAME is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValid(t)
			t.Setenv(tc.key, tc.val)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
			if strings.Contains(err.Error(), "super-secret-token") {
				t.Fatalf("token leaked: %v", err)
			}
		})
	}
}

func TestLoadWebhookRequiresSecret(t *testing.T) {
	setValid(t)
	t.Setenv("TELEGRAM_MODE", "webhook")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET is required") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	setValid(t)
	t.Setenv("TELEGRAM_BOT_USERNAME", "@secret_santa_bot")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramMode != ModePolling {
		t.Fatalf("mode %q", cfg.TelegramMode)
	}
	if cfg.TelegramBotUsername != "secret_santa_bot" {
		t.Fatalf("username %q", cfg.TelegramBotUsername)
	}
	if cfg.HTTPAddr != ":8080" || cfg.LogLevel != "info" {
		t.Fatalf("addr %q level %q", cfg.HTTPAddr, cfg.LogLevel)
	}
	if cfg.OutboxPollInterval != time.Second || cfg.SessionTTL != 30*time.Minute {
		t.Fatalf("interval %s ttl %s", cfg.OutboxPollInterval, cfg.SessionTTL)
	}
	if cfg.OutboxBatchSize != 20 {
		t.Fatalf("batch %d", cfg.OutboxBatchSize)
	}
	if cfg.TelegramWebhookSecret != "" {
		t.Fatal("secret should be empty in polling mode")
	}
}

func TestLoadForMigrateIgnoresMissingToken(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")

	cfg, err := LoadForMigrate()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/secretsanta" {
		t.Fatalf("url %q", cfg.DatabaseURL)
	}
	if cfg.TelegramBotToken != "" {
		t.Fatal("migrate config should not require a token")
	}
}

func TestLoadReadsDotEnvWithoutOverridingEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "from-env")

	dir := t.TempDir()
	contents := "DATABASE_URL=postgres://from-file/db\nTELEGRAM_BOT_TOKEN=from-file\nTELEGRAM_BOT_USERNAME=secret_santa_bot\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://from-file/db" {
		t.Fatalf("database url %q", cfg.DatabaseURL)
	}
	if cfg.TelegramBotToken != "from-env" {
		t.Fatalf("token overridden: %q", cfg.TelegramBotToken)
	}
	if cfg.TelegramBotUsername != "secret_santa_bot" {
		t.Fatalf("username %q", cfg.TelegramBotUsername)
	}
}

func TestApplyEnvFileParsesQuotesAndComments(t *testing.T) {
	clearEnv(t)

	err := applyEnvFile(strings.NewReader(`
# comment
export DATABASE_URL="postgres://from-file/db"
TELEGRAM_BOT_USERNAME='secret_santa_bot'
not a pair
`))
	if err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("DATABASE_URL"); got != "postgres://from-file/db" {
		t.Fatalf("database url %q", got)
	}
	if got := os.Getenv("TELEGRAM_BOT_USERNAME"); got != "secret_santa_bot" {
		t.Fatalf("username %q", got)
	}
}

func TestLoadForAdmin(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")
	t.Setenv("TELEGRAM_BOT_USERNAME", "@secret_santa_bot")
	t.Setenv("ADMIN_USERNAME", "operator")
	t.Setenv("ADMIN_PASSWORD", "test-password")
	t.Setenv("ADMIN_COOKIE_SECURE", "")
	t.Setenv("ADMIN_HTTP_ADDR", "")

	cfg, err := LoadForAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramBotUsername != "secret_santa_bot" || cfg.AdminUsername != "operator" {
		t.Fatalf("username %q admin %q", cfg.TelegramBotUsername, cfg.AdminUsername)
	}
	if cfg.AdminPassword != "test-password" {
		t.Fatal("password was not kept")
	}
	if cfg.AdminHTTPAddr != ":8080" || cfg.AdminCookieSecure || cfg.TelegramBotToken != "" {
		t.Fatalf("addr %q secure %v token %q", cfg.AdminHTTPAddr, cfg.AdminCookieSecure, cfg.TelegramBotToken)
	}
}

func TestLoadForAdminRejectsShortPassword(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")
	t.Setenv("TELEGRAM_BOT_USERNAME", "secret_santa_bot")
	t.Setenv("ADMIN_USERNAME", "operator")
	t.Setenv("ADMIN_PASSWORD", "secret1")

	_, err := LoadForAdmin()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "secret1") {
		t.Fatalf("password leaked: %v", err)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DATABASE_URL",
		"TELEGRAM_BOT_TOKEN",
		"TELEGRAM_BOT_USERNAME",
		"TELEGRAM_MODE",
		"TELEGRAM_WEBHOOK_SECRET",
		"HTTP_ADDR",
		"LOG_LEVEL",
		"OUTBOX_POLL_INTERVAL",
		"OUTBOX_BATCH_SIZE",
		"SESSION_TTL",
	} {
		t.Setenv(key, "")
	}
}

func setValid(t *testing.T) {
	t.Helper()
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/secretsanta")
	t.Setenv("TELEGRAM_BOT_TOKEN", "super-secret-token")
	t.Setenv("TELEGRAM_BOT_USERNAME", "secret_santa_bot")
}
