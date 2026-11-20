//go:build integration

package groups

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAndJoin(t *testing.T) {
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
	directory := NewPostgres(pool)

	created, err := directory.Create(ctx, owner, "  Друзья ")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Друзья" || created.MemberCount != 1 || len(created.Token) != 22 {
		t.Fatalf("created %+v", created)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM group_invites WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM group_members WHERE group_id = $1`, created.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, created.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, owner, guest)
	})

	var role string
	var uses int
	err = pool.QueryRow(ctx, `
		SELECT m.role, i.used_count
		FROM group_members m
		JOIN group_invites i ON i.group_id = m.group_id
		WHERE m.group_id = $1 AND m.user_id = $2
	`, created.ID, owner).Scan(&role, &uses)
	if err != nil {
		t.Fatal(err)
	}
	if role != "OWNER" || uses != 0 {
		t.Fatalf("role %s uses %d", role, uses)
	}

	preview, err := directory.Preview(ctx, created.Token)
	if err != nil || preview.GroupName != "Друзья" || preview.GroupID != created.ID {
		t.Fatalf("preview %+v err %v", preview, err)
	}
	if _, err := directory.Preview(ctx, "not-a-real-token"); err != ErrInviteInvalid {
		t.Fatalf("bad preview %v", err)
	}

	first, err := directory.Accept(ctx, guest, preview.InviteID)
	if err != nil || first.AlreadyMember || first.MemberCount != 2 || first.GroupName != "Друзья" {
		t.Fatalf("join %+v err %v", first, err)
	}
	second, err := directory.Accept(ctx, guest, preview.InviteID)
	if err != nil || !second.AlreadyMember || second.MemberCount != 2 {
		t.Fatalf("second %+v err %v", second, err)
	}
	err = pool.QueryRow(ctx, `SELECT used_count FROM group_invites WHERE id = $1`, preview.InviteID).Scan(&uses)
	if err != nil || uses != 1 {
		t.Fatalf("uses %d err %v", uses, err)
	}

	list, err := directory.List(ctx, guest)
	if err != nil || len(list) != 0 {
		t.Fatalf("member groups %+v err %v", list, err)
	}
	ownerList, err := directory.List(ctx, owner)
	if err != nil || len(ownerList) != 1 || ownerList[0].Name != "Друзья" || ownerList[0].Role != "OWNER" {
		t.Fatalf("owner groups %+v err %v", ownerList, err)
	}
	guestView, err := directory.Members(ctx, guest, created.ID)
	if err != nil || len(guestView) != 1 || guestView[0].Role != "OWNER" || guestView[0].DisplayName != "Анна" {
		t.Fatalf("guest members %+v err %v", guestView, err)
	}
	ownerView, err := directory.Members(ctx, owner, created.ID)
	if err != nil || len(ownerView) != 2 {
		t.Fatalf("owner members %+v err %v", ownerView, err)
	}
	opened, err := directory.Open(ctx, guest, created.ID)
	if err != nil || opened.OwnerName != "Анна" || opened.MemberCount != 2 || opened.Role != "MEMBER" {
		t.Fatalf("guest card %+v err %v", opened, err)
	}
	if _, err := directory.Members(ctx, owner+99, created.ID); err != ErrNotMember {
		t.Fatalf("stranger err %v", err)
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
