// Package telegram is a thin Telegram Bot API client.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Bot is the identity returned by getMe.
type Bot struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Client calls the Bot API. The token is kept in memory and redacted from errors.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API origin. Tests use this.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.http = client
		}
	}
}

// NewClient builds a client for token. token must already be non-empty.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: "https://api.telegram.org",
		token:   token,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	return c
}

// GetMe checks the bot token and returns the bot identity.
func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	if strings.TrimSpace(c.token) == "" {
		return Bot{}, fmt.Errorf("telegram getMe: token is empty")
	}

	endpoint := c.baseURL + "/bot" + c.token + "/getMe"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Bot{}, fail(c.token, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Bot{}, fail(c.token, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Bot{}, fail(c.token, err)
	}

	var payload struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Bot{}, fmt.Errorf("telegram getMe: status %d", resp.StatusCode)
	}
	if !payload.OK {
		if payload.Description == "" {
			return Bot{}, fmt.Errorf("telegram getMe: status %d", resp.StatusCode)
		}
		return Bot{}, fmt.Errorf("telegram getMe: %s", payload.Description)
	}

	var bot Bot
	if err := json.Unmarshal(payload.Result, &bot); err != nil {
		return Bot{}, fmt.Errorf("telegram getMe: decode result: %w", err)
	}
	if bot.ID == 0 {
		return Bot{}, fmt.Errorf("telegram getMe: missing bot id")
	}
	return bot, nil
}

// GetUpdates long-polls the Bot API. timeoutSec is the Telegram timeout, not the HTTP deadline.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+10)*time.Second)
	defer cancel()

	payload := map[string]any{
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if offset > 0 {
		payload["offset"] = offset
	}
	var result []Update
	if err := c.call(ctx, "getUpdates", payload, &result, true); err != nil {
		return nil, err
	}
	return result, nil
}

// SendMessage sends text with an inline keyboard.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, keyboard Keyboard) error {
	return c.call(ctx, "sendMessage", map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": keyboardBody(keyboard),
	}, nil, false)
}

// SendMessageHTML sends text that may contain Telegram HTML markup.
func (c *Client) SendMessageHTML(ctx context.Context, chatID int64, text string, keyboard Keyboard) error {
	return c.call(ctx, "sendMessage", map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"parse_mode":   "HTML",
		"reply_markup": keyboardBody(keyboard),
	}, nil, false)
}

// APIError is a rejected Bot API call. Description is not safe to log when the request carried a secret.
type APIError struct {
	Method      string
	Status      int
	Description string
	RetryAfter  time.Duration
}

func (e *APIError) Error() string {
	if e.Description == "" {
		return fmt.Sprintf("telegram %s: status %d", e.Method, e.Status)
	}
	return fmt.Sprintf("telegram %s: %s", e.Method, e.Description)
}

// EditMessageText replaces a menu message in place.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string, keyboard Keyboard) error {
	return c.call(ctx, "editMessageText", map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"text":         text,
		"reply_markup": keyboardBody(keyboard),
	}, nil, false)
}

// AnswerCallbackQuery stops the button spinner. text is an optional toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackID, text string) error {
	payload := map[string]any{"callback_query_id": callbackID}
	if text != "" {
		payload["text"] = text
	}
	return c.call(ctx, "answerCallbackQuery", payload, nil, false)
}

func keyboardBody(keyboard Keyboard) map[string]any {
	rows := make([][]map[string]any, 0, len(keyboard.Rows))
	for _, row := range keyboard.Rows {
		out := make([]map[string]any, 0, len(row))
		for _, button := range row {
			item := map[string]any{"text": button.Text}
			if button.Copy != "" {
				item["copy_text"] = map[string]string{"text": button.Copy}
			} else {
				item["callback_data"] = button.Data
			}
			out = append(out, item)
		}
		rows = append(rows, out)
	}
	return map[string]any{"inline_keyboard": rows}
}

func (c *Client) call(ctx context.Context, method string, payload any, dest any, longPoll bool) error {
	if strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("telegram %s: token is empty", method)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram %s: %s", method, redactToken(c.token, err.Error()))
	}
	endpoint := c.baseURL + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: %s", method, redactToken(c.token, err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.http
	if longPoll {
		copied := *c.http
		copied.Timeout = 0
		httpClient = &copied
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %s", method, redactToken(c.token, err.Error()))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("telegram %s: %s", method, redactToken(c.token, err.Error()))
	}
	var envelope struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &APIError{Method: method, Status: resp.StatusCode}
	}
	if !envelope.OK {
		status := envelope.ErrorCode
		if status == 0 {
			status = resp.StatusCode
		}
		return &APIError{
			Method:      method,
			Status:      status,
			Description: envelope.Description,
			RetryAfter:  time.Duration(envelope.Parameters.RetryAfter) * time.Second,
		}
	}
	if dest != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, dest); err != nil {
			return fmt.Errorf("telegram %s: decode result", method)
		}
	}
	return nil
}

func fail(secret string, err error) error {
	return fmt.Errorf("telegram getMe: %s", redactToken(secret, err.Error()))
}

func redactToken(secret, msg string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "***")
}
