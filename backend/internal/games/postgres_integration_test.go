//go:build integration

package games_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateGameKeepsMembersPending(t *testing.T) {
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
	guest := insertUser(t, ctx, pool, base+1, "Борис")
	third := insertUser(t, ctx, pool, base+2, "Виктор")
	fourth := insertUser(t, ctx, pool, base+3, "Глеб")
	admin := insertUser(t, ctx, pool, base+4, "Дарья")
	directory := groups.NewPostgres(pool)
	catalog := games.NewPostgres(pool)

	created, err := directory.Create(ctx, owner, "Друзья")
	if err != nil {
		t.Fatal(err)
	}
	colleagues, err := directory.Create(ctx, owner, "Коллеги")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, groupID := range []int64{created.ID, colleagues.ID} {
			_, _ = pool.Exec(bg, `DELETE FROM game_participants WHERE game_id IN (SELECT id FROM games WHERE group_id = $1)`, groupID)
			_, _ = pool.Exec(bg, `DELETE FROM games WHERE group_id = $1`, groupID)
			_, _ = pool.Exec(bg, `DELETE FROM group_invites WHERE group_id = $1`, groupID)
			_, _ = pool.Exec(bg, `DELETE FROM group_members WHERE group_id = $1`, groupID)
			_, _ = pool.Exec(bg, `DELETE FROM groups WHERE id = $1`, groupID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []int64{owner, guest, third, fourth, admin})
	})

	preview, err := directory.Preview(ctx, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Accept(ctx, guest, preview.InviteID); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Create(ctx, guest, created.ID, games.Input{Name: "Игра"}); err != games.ErrForbidden {
		t.Fatalf("member create %v", err)
	}
	if _, err := catalog.Create(ctx, owner, created.ID, games.Input{Name: " "}); err != games.ErrInvalidName {
		t.Fatalf("blank name %v", err)
	}

	day, _, err := games.ParseDay("01.01.2030")
	if err != nil {
		t.Fatal(err)
	}
	end := games.EndOfDay(day)
	budget := 2000
	view, err := catalog.Create(ctx, owner, created.ID, games.Input{
		Name: "  Тайный Санта 2026 ", Budget: &budget, Deadline: &end, Exchange: "2026-12-25",
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "Тайный Санта 2026" || view.Mine != games.Pending || view.Confirmed != 0 || view.Participants != 2 || view.Exchange != "2026-12-25" || view.Budget == nil || *view.Budget != 2000 {
		t.Fatalf("view %+v", view)
	}
	if view.Deadline == nil || !view.Deadline.Equal(end) {
		t.Fatalf("deadline %v", view.Deadline)
	}
	assertStatus(t, ctx, pool, view.ID, guest, games.Pending)

	again, err := directory.Accept(ctx, guest, preview.InviteID)
	if err != nil || !again.AlreadyMember || again.GameName != "" {
		t.Fatalf("repeat %+v err %v", again, err)
	}
	assertStatus(t, ctx, pool, view.ID, guest, games.Pending)

	if _, err := catalog.Create(ctx, owner, created.ID, games.Input{Name: "Ещё одна"}); err != games.ErrCurrentGameExists {
		t.Fatalf("second game %v", err)
	}

	joined, err := directory.Accept(ctx, third, preview.InviteID)
	if err != nil || joined.AlreadyMember || joined.GameID != view.ID || joined.GameName != "Тайный Санта 2026" {
		t.Fatalf("auto join %+v err %v", joined, err)
	}
	assertStatus(t, ctx, pool, view.ID, third, games.Confirmed)
	assertStatus(t, ctx, pool, view.ID, owner, games.Pending)

	confirmed, err := catalog.Decide(ctx, owner, view.ID, games.Confirmed)
	if err != nil || confirmed.Mine != games.Confirmed || confirmed.Confirmed != 2 {
		t.Fatalf("confirm %+v err %v", confirmed, err)
	}
	declined, err := catalog.Decide(ctx, guest, view.ID, games.Declined)
	if err != nil || declined.Mine != games.Declined || declined.Confirmed != 2 || declined.Participants != 3 {
		t.Fatalf("decline %+v err %v", declined, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE games SET registration_deadline = now() - interval '1 minute' WHERE id = $1`, view.ID); err != nil {
		t.Fatal(err)
	}
	late, err := directory.Accept(ctx, fourth, preview.InviteID)
	if err != nil || late.GameName != "" {
		t.Fatalf("late %+v err %v", late, err)
	}
	var rows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM game_participants WHERE game_id = $1 AND user_id = $2`, view.ID, fourth).Scan(&rows)
	if err != nil || rows != 0 {
		t.Fatalf("late rows %d err %v", rows, err)
	}
	if _, err := catalog.Decide(ctx, owner, view.ID, games.Declined); err != games.ErrRegistrationClosed {
		t.Fatalf("closed %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id, role, status) VALUES ($1, $2, 'ADMIN', 'ACTIVE')
	`, colleagues.ID, admin); err != nil {
		t.Fatal(err)
	}
	adminGame, err := catalog.Create(ctx, admin, colleagues.ID, games.Input{Name: "Офис"})
	if err != nil || adminGame.Mine != games.Pending || adminGame.Participants != 2 {
		t.Fatalf("admin game %+v err %v", adminGame, err)
	}
}

func assertStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, gameID, userID int64, status string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
		SELECT status FROM game_participants WHERE game_id = $1 AND user_id = $2
	`, gameID, userID).Scan(&got)
	if err != nil || got != status {
		t.Fatalf("participant %d status %q err %v", userID, got, err)
	}
}

func insertUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, telegramID int64, name string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, display_name) VALUES ($1, $2) RETURNING id
	`, telegramID, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
