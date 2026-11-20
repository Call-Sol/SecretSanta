//go:build integration

package draws_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/draws"
	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/notifications"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunDrawQueuesPrivateMessages(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	base := time.Now().UnixNano()
	owner := insertUser(t, ctx, pool, base, "Анна")
	first := insertUser(t, ctx, pool, base+1, "Борис")
	second := insertUser(t, ctx, pool, base+2, "Виктор")
	waiting := insertUser(t, ctx, pool, base+3, "Глеб")
	directory := groups.NewPostgres(pool)
	catalog := games.NewPostgres(pool)
	runner := draws.NewPostgres(pool)

	created, err := directory.Create(ctx, owner, "Друзья")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `UPDATE games SET active_draw_id = NULL WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM notification_outbox WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM assignments WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM draws WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM game_participants WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM games WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM group_invites WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM group_members WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM groups WHERE id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []int64{owner, first, second, waiting})
	})

	preview, err := directory.Preview(ctx, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{first, second, waiting} {
		if _, err := directory.Accept(ctx, userID, preview.InviteID); err != nil {
			t.Fatal(err)
		}
	}
	day, _, err := games.ParseDay("25.12.2030")
	if err != nil {
		t.Fatal(err)
	}
	end := games.EndOfDay(day)
	view, err := catalog.Create(ctx, owner, created.ID, games.Input{
		Name: "Тайный Санта 2026", Description: "до 2000 ₽, красный", Deadline: &end, Exchange: "2030-12-25",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{owner, first, second} {
		if _, err := catalog.Decide(ctx, userID, view.ID, games.Confirmed); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Run(ctx, waiting, view.ID); err != draws.ErrForbidden {
		t.Fatalf("member draw %v", err)
	}

	var firstErr, secondErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		firstErr = runner.Run(ctx, owner, view.ID)
	}()
	go func() {
		defer wg.Done()
		secondErr = runner.Run(ctx, owner, view.ID)
	}()
	wg.Wait()
	if (firstErr == nil) == (secondErr == nil) || (firstErr != nil && firstErr != draws.ErrAlreadyDrawn) || (secondErr != nil && secondErr != draws.ErrAlreadyDrawn) {
		t.Fatalf("parallel %v %v", firstErr, secondErr)
	}
	if err := runner.Run(ctx, owner, view.ID); err != draws.ErrAlreadyDrawn {
		t.Fatalf("third %v", err)
	}

	var drawsCount, active int
	err = pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = 'ACTIVE')
		FROM draws WHERE game_id = $1
	`, view.ID).Scan(&drawsCount, &active)
	if err != nil || drawsCount != 1 || active != 1 {
		t.Fatalf("draws %d active %d err %v", drawsCount, active, err)
	}

	edges := map[int64]int64{}
	names := map[int64]string{}
	rows, err := pool.Query(ctx, `
		SELECT giver.user_id, receiver.user_id, ru.display_name
		FROM assignments a
		JOIN game_participants giver ON giver.id = a.giver_participant_id
		JOIN game_participants receiver ON receiver.id = a.receiver_participant_id
		JOIN users ru ON ru.id = receiver.user_id
		WHERE a.game_id = $1
	`, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var giver, receiver int64
		var name string
		if err := rows.Scan(&giver, &receiver, &name); err != nil {
			t.Fatal(err)
		}
		edges[giver] = receiver
		names[receiver] = name
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(edges) != 3 || edges[waiting] != 0 {
		t.Fatalf("edges %+v", edges)
	}
	seen := map[int64]bool{}
	current := owner
	for i := 0; i < 3; i++ {
		next, ok := edges[current]
		if !ok || seen[current] {
			t.Fatalf("cycle %+v at %d", edges, current)
		}
		seen[current] = true
		current = next
	}
	if current != owner || len(seen) != 3 {
		t.Fatalf("cycle %+v", edges)
	}

	for _, userID := range []int64{owner, first, second} {
		mine, err := runner.ViewMine(ctx, userID, view.ID)
		if err != nil || mine.ReceiverName != names[edges[userID]] || mine.ReceiverName == "" {
			t.Fatalf("mine user %d %+v err %v", userID, mine, err)
		}
		if strings.Contains(mine.ReceiverName, names[userID]) && edges[userID] == userID {
			t.Fatal("self assignment")
		}
	}
	if _, err := runner.ViewMine(ctx, waiting, view.ID); err != draws.ErrNotAssigned {
		t.Fatalf("pending %v", err)
	}

	var queued int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM notification_outbox
		WHERE game_id = $1 AND status = 'PENDING' AND event_type = 'ASSIGNMENT_CREATED'
	`, view.ID).Scan(&queued)
	if err != nil || queued != 3 {
		t.Fatalf("queued %d err %v", queued, err)
	}

	sent := &capture{}
	n, err := notifications.ProcessOnce(ctx, pool, sent, nil, 10)
	if err != nil || n != 3 || len(sent.texts) != 3 {
		t.Fatalf("sent %d texts %d err %v", n, len(sent.texts), err)
	}
	chats := map[int64]string{}
	for i, chatID := range sent.chats {
		chats[chatID] = sent.texts[i]
	}
	for _, person := range []struct {
		telegram int64
		user     int64
	}{{base, owner}, {base + 1, first}, {base + 2, second}} {
		body := chats[person.telegram]
		receiver := names[edges[person.user]]
		if !strings.Contains(body, "Ты даришь подарок: "+receiver) || strings.Contains(body, "<b>") || !strings.Contains(body, "25 декабря") || !strings.Contains(body, "до 2000 ₽, красный") {
			t.Fatalf("chat %d body %q receiver %s", person.telegram, body, receiver)
		}
	}
	var stillPending int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox WHERE game_id = $1 AND status <> 'SENT'`, view.ID).Scan(&stillPending)
	if err != nil || stillPending != 0 {
		t.Fatalf("unsent %d err %v", stillPending, err)
	}
}

func TestTwoConfirmedPeopleCannotDraw(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	base := time.Now().UnixNano()
	owner := insertUser(t, ctx, pool, base+10, "Анна")
	guest := insertUser(t, ctx, pool, base+11, "Борис")
	directory := groups.NewPostgres(pool)
	created, err := directory.Create(ctx, owner, "Пара")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM game_participants WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM games WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM group_invites WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM group_members WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM groups WHERE id = $1`, created.ID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []int64{owner, guest})
	})
	preview, err := directory.Preview(ctx, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Accept(ctx, guest, preview.InviteID); err != nil {
		t.Fatal(err)
	}
	view, err := games.NewPostgres(pool).Create(ctx, owner, created.ID, games.Input{Name: "Двое"})
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{owner, guest} {
		if _, err := games.NewPostgres(pool).Decide(ctx, userID, view.ID, games.Confirmed); err != nil {
			t.Fatal(err)
		}
	}
	if err := draws.NewPostgres(pool).Run(ctx, owner, view.ID); err != draws.ErrTooFewParticipants {
		t.Fatalf("short draw %v", err)
	}
}

type capture struct {
	chats []int64
	texts []string
}

func (c *capture) SendAssignment(_ context.Context, chatID int64, text string, gameID int64) error {
	if gameID == 0 {
		return context.Canceled
	}
	c.chats = append(c.chats, chatID)
	c.texts = append(c.texts, text)
	return nil
}

func insertUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, telegramID int64, name string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, `INSERT INTO users (telegram_id, display_name) VALUES ($1, $2) RETURNING id`, telegramID, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
