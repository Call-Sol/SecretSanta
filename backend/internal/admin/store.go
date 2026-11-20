package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence used by the operator panel.
type Store interface {
	CreateSession(ctx context.Context, ttl time.Duration) (rawToken string, csrf string, err error)
	DeleteSession(ctx context.Context, rawToken string) error
	Session(ctx context.Context, rawToken string) (Session, error)

	CreateGroup(ctx context.Context, name string) (Group, error)
	ListGroups(ctx context.Context) ([]GroupSummary, error)
	ListUsers(ctx context.Context) ([]RegisteredUser, error)
	Group(ctx context.Context, id int64) (GroupDetail, error)
	ArchiveGroup(ctx context.Context, id int64) error
	CreateInvite(ctx context.Context, groupID int64, opts InviteOptions) error
	RevokeInvite(ctx context.Context, inviteID int64) (groupID int64, err error)
}

// Postgres implements Store with pgx.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a store backed by pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (s *Postgres) CreateSession(ctx context.Context, ttl time.Duration) (string, string, error) {
	raw, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO admin_sessions (token_hash, csrf_token, expires_at)
		VALUES ($1, $2, $3)
	`, hashToken(raw), csrf, time.Now().Add(ttl))
	if err != nil {
		return "", "", fmt.Errorf("create session: %w", err)
	}
	return raw, csrf, nil
}

func (s *Postgres) DeleteSession(ctx context.Context, rawToken string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash = $1`, hashToken(rawToken))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Postgres) Session(ctx context.Context, rawToken string) (Session, error) {
	_, _ = s.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at <= now()`)
	var sess Session
	err := s.pool.QueryRow(ctx, `
		SELECT csrf_token, expires_at
		FROM admin_sessions
		WHERE token_hash = $1 AND expires_at > now()
	`, hashToken(rawToken)).Scan(&sess.CSRF, &sess.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	return sess, nil
}

func (s *Postgres) CreateGroup(ctx context.Context, name string) (Group, error) {
	name, err := NormalizeGroupName(name)
	if err != nil {
		return Group{}, err
	}
	var g Group
	err = s.pool.QueryRow(ctx, `
		INSERT INTO groups (name, status)
		VALUES ($1, 'ACTIVE')
		RETURNING id, name, status, created_at
	`, name).Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt)
	if err != nil {
		return Group{}, fmt.Errorf("create group: %w", err)
	}
	return g, nil
}

func (s *Postgres) ListGroups(ctx context.Context) ([]GroupSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, g.status, g.created_at,
			(SELECT count(*) FROM group_members m
			  WHERE m.group_id = g.id AND m.status = 'ACTIVE'),
			(SELECT count(*) FROM group_invites i
			  WHERE i.group_id = g.id
			    AND i.revoked_at IS NULL
			    AND (i.expires_at IS NULL OR i.expires_at > now())
			    AND (i.max_uses IS NULL OR i.used_count < i.max_uses))
		FROM groups g
		ORDER BY g.created_at DESC, g.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var out []GroupSummary
	for rows.Next() {
		var item GroupSummary
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Status, &item.CreatedAt,
			&item.MemberCount, &item.ActiveInvites,
		); err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return out, nil
}

func (s *Postgres) ListUsers(ctx context.Context) ([]RegisteredUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.telegram_id, u.username, u.display_name, u.dm_started_at, u.created_at,
			(SELECT count(*) FROM group_members m
			  WHERE m.user_id = u.id AND m.status = 'ACTIVE')
		FROM users u
		ORDER BY u.created_at DESC, u.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []RegisteredUser
	for rows.Next() {
		var item RegisteredUser
		var username *string
		if err := rows.Scan(
			&item.ID, &item.TelegramID, &username, &item.DisplayName,
			&item.DMStartedAt, &item.CreatedAt, &item.GroupCount,
		); err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		if username != nil {
			item.Username = *username
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return out, nil
}

func (s *Postgres) Group(ctx context.Context, id int64) (GroupDetail, error) {
	var detail GroupDetail
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, status, created_at FROM groups WHERE id = $1
	`, id).Scan(&detail.ID, &detail.Name, &detail.Status, &detail.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupDetail{}, ErrNotFound
	}
	if err != nil {
		return GroupDetail{}, fmt.Errorf("get group: %w", err)
	}

	members, err := s.pool.Query(ctx, `
		SELECT u.display_name, u.username, m.role, m.status, m.joined_at
		FROM group_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = $1
		ORDER BY m.joined_at, u.id
	`, id)
	if err != nil {
		return GroupDetail{}, fmt.Errorf("list members: %w", err)
	}
	defer members.Close()
	for members.Next() {
		var member Member
		var username *string
		if err := members.Scan(&member.DisplayName, &username, &member.Role, &member.Status, &member.JoinedAt); err != nil {
			return GroupDetail{}, fmt.Errorf("list members: %w", err)
		}
		if username != nil {
			member.Username = *username
		}
		detail.Members = append(detail.Members, member)
	}
	if err := members.Err(); err != nil {
		return GroupDetail{}, fmt.Errorf("list members: %w", err)
	}

	invites, err := s.pool.Query(ctx, `
		SELECT id, created_at, expires_at, max_uses, used_count, revoked_at, token
		FROM group_invites
		WHERE group_id = $1
		ORDER BY id DESC
	`, id)
	if err != nil {
		return GroupDetail{}, fmt.Errorf("list invites: %w", err)
	}
	defer invites.Close()
	now := time.Now()
	for invites.Next() {
		var inv Invite
		var token *string
		if err := invites.Scan(&inv.ID, &inv.CreatedAt, &inv.ExpiresAt, &inv.MaxUses, &inv.UsedCount, &inv.RevokedAt, &token); err != nil {
			return GroupDetail{}, fmt.Errorf("list invites: %w", err)
		}
		if token != nil {
			inv.Token = *token
		}
		inv.decorate(now)
		detail.Invites = append(detail.Invites, inv)
	}
	if err := invites.Err(); err != nil {
		return GroupDetail{}, fmt.Errorf("list invites: %w", err)
	}
	return detail, nil
}

func (s *Postgres) ArchiveGroup(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE groups
		SET status = 'ARCHIVED', archived_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'ACTIVE'
	`, id)
	if err != nil {
		return fmt.Errorf("archive group: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status FROM groups WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive group: %w", err)
	}
	return nil
}

func (s *Postgres) CreateInvite(ctx context.Context, groupID int64, opts InviteOptions) error {
	raw, tokenHash, err := newInviteToken()
	if err != nil {
		return err
	}
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "ACTIVE" {
			return ErrArchived
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO group_invites (group_id, token_hash, token, expires_at, max_uses)
			VALUES ($1, $2, $3, $4, $5)
		`, groupID, tokenHash, raw, opts.ExpiresAt, opts.MaxUses)
		return err
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrArchived) {
		return err
	}
	if err != nil {
		return fmt.Errorf("create invite failed")
	}
	return nil
}

func (s *Postgres) RevokeInvite(ctx context.Context, inviteID int64) (int64, error) {
	var groupID int64
	err := s.pool.QueryRow(ctx, `
		UPDATE group_invites
		SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL
		RETURNING group_id
	`, inviteID).Scan(&groupID)
	if err == nil {
		return groupID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("revoke invite: %w", err)
	}
	err = s.pool.QueryRow(ctx, `SELECT group_id FROM group_invites WHERE id = $1`, inviteID).Scan(&groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("revoke invite: %w", err)
	}
	return groupID, nil
}
