// Package config loads process configuration from the environment.
// A .env file in the working directory fills in variables that are not already set.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mode is how the bot receives Telegram updates.
type Mode string

const (
	ModePolling Mode = "polling"
	ModeWebhook Mode = "webhook"
)

// Config is the process configuration. The bot token is never logged.
type Config struct {
	DatabaseURL           string
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramMode          Mode
	TelegramWebhookSecret string
	HTTPAddr              string
	LogLevel              string
	OutboxPollInterval    time.Duration
	OutboxBatchSize       int
	SessionTTL            time.Duration
}

// Load reads a full bot or worker configuration.
// DATABASE_URL, TELEGRAM_BOT_TOKEN, and TELEGRAM_BOT_USERNAME are required.
func Load() (Config, error) {
	loadDotEnv()
	return load(true)
}

// LoadForMigrate reads only the settings the migration command needs.
func LoadForMigrate() (Config, error) {
	loadDotEnv()
	return load(false)
}

func load(full bool) (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	cfg := Config{DatabaseURL: databaseURL}
	if !full {
		return cfg, nil
	}

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is invalid")
	}

	username := strings.TrimPrefix(strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME")), "@")
	if username == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_USERNAME is required")
	}
	if strings.ContainsAny(username, " /?#") {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_USERNAME is invalid")
	}

	mode, err := parseMode(getenv("TELEGRAM_MODE", string(ModePolling)))
	if err != nil {
		return Config{}, err
	}
	level, err := parseLogLevel(getenv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	interval, err := parsePositiveDuration("OUTBOX_POLL_INTERVAL", getenv("OUTBOX_POLL_INTERVAL", "1s"))
	if err != nil {
		return Config{}, err
	}
	ttl, err := parsePositiveDuration("SESSION_TTL", getenv("SESSION_TTL", "30m"))
	if err != nil {
		return Config{}, err
	}
	batch, err := parsePositiveInt("OUTBOX_BATCH_SIZE", getenv("OUTBOX_BATCH_SIZE", "20"))
	if err != nil {
		return Config{}, err
	}

	secret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if mode == ModeWebhook && secret == "" {
		return Config{}, fmt.Errorf("TELEGRAM_WEBHOOK_SECRET is required")
	}

	cfg.TelegramBotToken = token
	cfg.TelegramBotUsername = username
	cfg.TelegramMode = mode
	cfg.TelegramWebhookSecret = secret
	cfg.HTTPAddr = getenv("HTTP_ADDR", ":8080")
	cfg.LogLevel = level
	cfg.OutboxPollInterval = interval
	cfg.OutboxBatchSize = batch
	cfg.SessionTTL = ttl
	return cfg, nil
}

func parseMode(raw string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(raw))) {
	case ModePolling:
		return ModePolling, nil
	case ModeWebhook:
		return ModeWebhook, nil
	default:
		return "", fmt.Errorf("TELEGRAM_MODE must be polling or webhook")
	}
}

func parseLogLevel(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	default:
		return "", fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
}

func parsePositiveDuration(name, raw string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return d, nil
}

func parsePositiveInt(name, raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	_ = applyEnvFile(f)
}

// applyEnvFile sets unset environment variables from a dotenv document.
func applyEnvFile(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return sc.Err()
}
