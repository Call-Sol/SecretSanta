//go:build integration

package admin

import (
	"bytes"
	"context"
	"os"
	"strings"
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

func TestPostgresGamesHideAssignments(t *testing.T) {
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

	group, err := store.CreateGroup(ctx, "Panel games")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UnixNano()
	var anna, boris int64
	err = pool.QueryRow(ctx, `INSERT INTO users (telegram_id, display_name, username) VALUES ($1, 'Anna', 'anna') RETURNING id`, base).Scan(&anna)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO users (telegram_id, display_name) VALUES ($1, 'Boris') RETURNING id`, base+1).Scan(&boris)
	if err != nil {
		t.Fatal(err)
	}
	token := "panel-game-invite-1"
	var gameID, drawID, giverID, receiverID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO games (group_id, name, status, description, registration_deadline, exchange_on, invite_token, invite_token_hash)
		VALUES ($1, 'Office party', 'REGISTRATION', 'red paper', $2, '2026-12-25', $3, $4)
		RETURNING id
	`, group.ID, time.Date(2026, 12, 2, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60)), token, hashToken(token)).Scan(&gameID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `UPDATE games SET active_draw_id = NULL WHERE id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM notification_outbox WHERE game_id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM assignments WHERE game_id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM draws WHERE game_id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM game_participants WHERE game_id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM games WHERE id = $1`, gameID)
		_, _ = pool.Exec(bg, `DELETE FROM group_invites WHERE group_id = $1`, group.ID)
		_, _ = pool.Exec(bg, `DELETE FROM group_members WHERE group_id = $1`, group.ID)
		_, _ = pool.Exec(bg, `DELETE FROM groups WHERE id = $1`, group.ID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []int64{anna, boris})
	})
	err = pool.QueryRow(ctx, `
		INSERT INTO game_participants (game_id, user_id, status, assignment_state)
		VALUES ($1, $2, 'CONFIRMED', 'ASSIGNED') RETURNING id
	`, gameID, anna).Scan(&giverID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO game_participants (game_id, user_id, status, assignment_state)
		VALUES ($1, $2, 'PENDING', 'NOT_APPLICABLE') RETURNING id
	`, gameID, boris).Scan(&receiverID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO draws (game_id, version, status, created_by) VALUES ($1, 1, 'ACTIVE', $2) RETURNING id
	`, gameID, anna).Scan(&drawID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE games SET status = 'DRAWN', active_draw_id = $2 WHERE id = $1`, gameID, drawID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO assignments (draw_id, game_id, giver_participant_id, receiver_participant_id)
		VALUES ($1, $2, $3, $4)
	`, drawID, gameID, giverID, receiverID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO notification_outbox (user_id, game_id, draw_id, event_type, payload, status)
		VALUES ($1, $2, $3, 'ASSIGNMENT_CREATED', '{"receiver_name":"HIDDEN-GIFT"}', 'PENDING')
	`, anna, gameID, drawID)
	if err != nil {
		t.Fatal(err)
	}

	listed, err := store.ListGames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range listed {
		if item.ID == gameID {
			found = true
			if item.Name != "Office party" || item.GroupName != "Panel games" || item.Status != "DRAWN" || item.Confirmed != 1 || item.ParticipantCount != 2 {
				t.Fatalf("summary status %s confirmed %d of %d", item.Status, item.Confirmed, item.ParticipantCount)
			}
		}
	}
	if !found {
		t.Fatal("game missing from list")
	}
	detail, err := store.Game(ctx, gameID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Description != "red paper" || detail.Exchange != "2026-12-25" || detail.InviteToken != token || len(detail.People) != 2 {
		t.Fatal("game detail mismatch")
	}
	if detail.People[0].DisplayName != "Anna" || detail.People[0].Status != "CONFIRMED" || detail.People[0].AssignmentState != "ASSIGNED" {
		t.Fatalf("anna %s %s", detail.People[0].Status, detail.People[0].AssignmentState)
	}
	if detail.People[1].DisplayName != "Boris" || detail.People[1].Status != "PENDING" {
		t.Fatalf("boris %s", detail.People[1].Status)
	}
	visible := detail.Name + detail.Description + detail.Exchange
	for _, person := range detail.People {
		visible += person.DisplayName + person.Username + person.Status + person.AssignmentState
	}
	if strings.Contains(visible, "HIDDEN-GIFT") {
		t.Fatal("assignment payload was loaded")
	}
	if _, err := store.Game(ctx, gameID+1000000); err != ErrNotFound {
		t.Fatalf("missing game %v", err)
	}
}
