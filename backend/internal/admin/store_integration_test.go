//go:build integration

package admin

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
)

func TestPostgresGroupsAndInvites(t *testing.T) {
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
	store := NewPostgres(pool)

	group, err := store.CreateGroup(ctx, " Ops friends ")
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "Ops friends" || group.Status != "ACTIVE" {
		t.Fatalf("group %+v", group)
	}

	var userID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, display_name, username)
		VALUES ($1, 'Anna', 'anna')
		RETURNING id
	`, time.Now().UnixNano()).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id, role, status)
		VALUES ($1, $2, 'MEMBER', 'ACTIVE')
	`, group.ID, userID)
	if err != nil {
		t.Fatal(err)
	}

	limit := 2
	expires := time.Now().Add(48 * time.Hour).UTC()
	if err := store.CreateInvite(ctx, group.ID, InviteOptions{MaxUses: &limit, ExpiresAt: &expires}); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	var token string
	err = pool.QueryRow(ctx, `SELECT token_hash, token FROM group_invites WHERE group_id = $1`, group.ID).Scan(&stored, &token)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || !bytes.Equal(stored, hashToken(token)) || bytes.Contains(stored, []byte(token)) {
		t.Fatal("invite token was not stored with its hash")
	}
	detail, err := store.Group(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Invites) != 1 || detail.Invites[0].Token != token {
		t.Fatalf("invite token not readable: %+v", detail.Invites)
	}

	listed, err := store.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range listed {
		if item.ID == group.ID {
			found = true
			if item.MemberCount != 1 || item.ActiveInvites != 1 {
				t.Fatalf("counts members %d invites %d", item.MemberCount, item.ActiveInvites)
			}
		}
	}
	if !found {
		t.Fatal("group missing from list")
	}

	detail, err = store.Group(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 1 || detail.Members[0].DisplayName != "Anna" || detail.Members[0].Username != "anna" {
		t.Fatalf("members %+v", detail.Members)
	}
	people, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var registered *RegisteredUser
	for i := range people {
		if people[i].ID == userID {
			registered = &people[i]
			break
		}
	}
	if registered == nil || registered.DisplayName != "Anna" || registered.Username != "anna" || registered.GroupCount != 1 {
		t.Fatalf("registered user %+v", registered)
	}
	if len(detail.Invites) != 1 || detail.Invites[0].State != "Active" || detail.Invites[0].Usage != "0 / 2" {
		t.Fatalf("invites %+v", detail.Invites)
	}
	if _, err := store.RevokeInvite(ctx, detail.Invites[0].ID); err != nil {
		t.Fatal(err)
	}
	detail, err = store.Group(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Invites[0].State != "Revoked" {
		t.Fatalf("state %s", detail.Invites[0].State)
	}

	if err := store.ArchiveGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateInvite(ctx, group.ID, InviteOptions{}); err != ErrArchived {
		t.Fatalf("archived invite err %v", err)
	}
}
