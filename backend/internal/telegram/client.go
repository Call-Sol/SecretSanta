// Package telegram is a thin Telegram Bot API client.
package telegram

import (
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

func fail(secret string, err error) error {
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	return fmt.Errorf("telegram getMe: %s", msg)
}
