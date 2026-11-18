package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsuper-secret-token/getMe" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"Secret Santa","username":"secret_santa_bot"}}`))
	}))
	t.Cleanup(srv.Close)

	bot, err := NewClient("super-secret-token", WithBaseURL(srv.URL)).GetMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bot.ID != 42 || bot.Username != "secret_santa_bot" || !bot.IsBot {
		t.Fatalf("bot %+v", bot)
	}
}

func TestGetMeUnauthorizedDoesNotEchoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := NewClient("super-secret-token", WithBaseURL(srv.URL)).GetMe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestGetMeRedactsTokenOnDialError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewClient("super-secret-token", WithBaseURL("http://127.0.0.1:1")).GetMe(ctx)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestGetMeRejectsEmptyToken(t *testing.T) {
	_, err := NewClient("  ").GetMe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token is empty") {
		t.Fatalf("got %v", err)
	}
}
