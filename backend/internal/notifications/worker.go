package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/draws"
	"github.com/alekseymakarkin/secret-santa/internal/telegram"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxAttempts = 8

// Sender delivers one private assignment. The text is already rendered.
type Sender interface {
	SendAssignment(ctx context.Context, chatID int64, text string, gameID int64) error
}

// ClientSender posts assignment messages through the Bot API.
type ClientSender struct {
	Client *telegram.Client
}

// SendAssignment sends the HTML message and a button that opens the same result again.
func (s ClientSender) SendAssignment(ctx context.Context, chatID int64, text string, gameID int64) error {
	var keyboard telegram.Keyboard
	if gameID > 0 {
		keyboard = telegram.Keyboard{Rows: [][]telegram.Button{
			{{Text: "Посмотреть получателя", Data: "game:mine:" + strconv.FormatInt(gameID, 10)}},
		}}
	}
	return s.Client.SendMessageHTML(ctx, chatID, text, keyboard)
}

// Run sends due outbox rows until ctx is cancelled.
func Run(ctx context.Context, pool *pgxpool.Pool, sender Sender, log *slog.Logger, interval time.Duration, batch int) error {
	if log == nil {
		log = slog.Default()
	}
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 20
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := ProcessOnce(ctx, pool, sender, log, batch)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Error("outbox", "err", err.Error())
		}
		if n > 0 {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

type job struct {
	ID       int64
	UserID   int64
	ChatID   int64
	GameID   int64
	Payload  []byte
	Attempts int
}

// ProcessOnce claims a batch and sends it. It returns how many rows were claimed.
func ProcessOnce(ctx context.Context, pool *pgxpool.Pool, sender Sender, log *slog.Logger, batch int) (int, error) {
	if log == nil {
		log = slog.Default()
	}
	if _, err := pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'PENDING', claimed_at = NULL
		WHERE status = 'SENDING' AND claimed_at < now() - interval '2 minutes'
	`); err != nil {
		return 0, err
	}
	rows, err := pool.Query(ctx, `
		UPDATE notification_outbox
		SET status = 'SENDING', attempts = attempts + 1, claimed_at = now()
		WHERE id IN (
			SELECT id FROM notification_outbox
			WHERE status = 'PENDING' AND next_attempt_at <= now()
			ORDER BY next_attempt_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id, user_id, game_id, payload, attempts
	`, batch)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var jobs []job
	for rows.Next() {
		var item job
		var gameID *int64
		if err := rows.Scan(&item.ID, &item.UserID, &gameID, &item.Payload, &item.Attempts); err != nil {
			return 0, err
		}
		if gameID != nil {
			item.GameID = *gameID
		}
		jobs = append(jobs, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	chats, err := chatIDs(ctx, pool, jobs)
	if err != nil {
		return len(jobs), err
	}
	for i := range jobs {
		jobs[i].ChatID = chats[jobs[i].UserID]
		deliver(ctx, pool, sender, log, jobs[i])
	}
	return len(jobs), nil
}

func chatIDs(ctx context.Context, pool *pgxpool.Pool, jobs []job) (map[int64]int64, error) {
	ids := make([]int64, len(jobs))
	for i, item := range jobs {
		ids[i] = item.UserID
	}
	rows, err := pool.Query(ctx, `SELECT id, telegram_id FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, telegramID int64
		if err := rows.Scan(&id, &telegramID); err != nil {
			return nil, err
		}
		out[id] = telegramID
	}
	return out, rows.Err()
}

func deliver(ctx context.Context, pool *pgxpool.Pool, sender Sender, log *slog.Logger, item job) {
	if item.Attempts > maxAttempts {
		mark(ctx, pool, item.ID, "FAILED", time.Time{}, "too many attempts")
		log.Info("outbox failed", "id", item.ID, "user_id", item.UserID, "class", "too many attempts")
		return
	}
	var payload draws.AssignmentPayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil || payload.ReceiverName == "" || item.ChatID == 0 {
		mark(ctx, pool, item.ID, "FAILED", time.Time{}, "bad payload")
		log.Info("outbox failed", "id", item.ID, "user_id", item.UserID, "class", "bad payload")
		return
	}
	text := draws.FormatAssignment(payload.GameName, payload.ReceiverName, payload.ReceiverUsername, payload.Description, payload.ExchangeOn)
	err := sender.SendAssignment(ctx, item.ChatID, text, item.GameID)
	if err == nil {
		mark(ctx, pool, item.ID, "SENT", time.Time{}, "")
		log.Info("outbox sent", "id", item.ID, "user_id", item.UserID)
		return
	}
	if ctx.Err() != nil {
		return
	}
	failed, wait, class := classify(err, item.Attempts)
	if failed {
		mark(ctx, pool, item.ID, "FAILED", time.Time{}, class)
		log.Info("outbox failed", "id", item.ID, "user_id", item.UserID, "class", class)
		return
	}
	mark(ctx, pool, item.ID, "PENDING", time.Now().Add(wait), class)
	log.Info("outbox retry", "id", item.ID, "user_id", item.UserID, "class", class)
}

func classify(err error, attempts int) (bool, time.Duration, string) {
	var api *telegram.APIError
	if errors.As(err, &api) {
		if api.Status == 403 || api.Status == 400 {
			return true, 0, "blocked"
		}
		if api.Status == 429 {
			wait := api.RetryAfter
			if wait <= 0 {
				wait = backoff(attempts)
			}
			return false, wait, "rate limited"
		}
	}
	if attempts >= maxAttempts {
		return true, 0, "too many attempts"
	}
	return false, backoff(attempts), "telegram unavailable"
}

func backoff(attempts int) time.Duration {
	switch attempts {
	case 1:
		return 5 * time.Second
	case 2:
		return 15 * time.Second
	case 3:
		return time.Minute
	case 4:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

func mark(ctx context.Context, pool *pgxpool.Pool, id int64, status string, next time.Time, class string) {
	var last any
	if class != "" {
		last = class
	}
	var when any
	if !next.IsZero() {
		when = next
	}
	_, _ = pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = $2,
		    next_attempt_at = COALESCE($3, next_attempt_at),
		    last_error = $4,
		    sent_at = CASE WHEN $2 = 'SENT' THEN now() ELSE sent_at END,
		    claimed_at = CASE WHEN $2 = 'SENDING' THEN claimed_at ELSE NULL END
		WHERE id = $1
	`, id, status, when, last)
}
