package telegram

import (
	"context"
	"io"
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

func TestSendMessagePostsMenuKeyboard(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsuper-secret-token/sendMessage" {
			t.Errorf("path %s", r.URL.Path)
		}
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	t.Cleanup(srv.Close)

	err := NewClient("super-secret-token", WithBaseURL(srv.URL)).SendMessage(context.Background(), 42, menuText, Keyboard{Rows: [][]Button{
		mainKeyboard().Rows[0],
		{{Text: "Пригласить", Copy: "https://t.me/secret_santa_bot?start=join_abc"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, part := range []string{`"chat_id":42`, "Мои игры", "game:list", `"copy_text":{"text":"https://t.me/secret_santa_bot?start=join_abc"}`} {
		if !strings.Contains(text, part) {
			t.Fatalf("body missing %s: %s", part, text)
		}
	}
	if strings.Contains(text, "super-secret-token") {
		t.Fatalf("token leaked: %s", text)
	}
}

func TestGetMeRejectsEmptyToken(t *testing.T) {
	_, err := NewClient("  ").GetMe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token is empty") {
		t.Fatalf("got %v", err)
	}
}
