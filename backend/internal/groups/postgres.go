package groups

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres stores groups, membership, and invites.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a directory backed by pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Create makes an active group, an owner membership, and one invite.
func (s *Postgres) Create(ctx context.Context, userID int64, name string) (Created, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return Created{}, err
	}
	token, hash, err := newInviteToken()
	if err != nil {
		return Created{}, err
	}
	var created Created
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO groups (name, owner_id, status)
			VALUES ($1, $2, 'ACTIVE')
			RETURNING id
		`, name, userID).Scan(&created.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO group_members (group_id, user_id, role, status)
			VALUES ($1, $2, 'OWNER', 'ACTIVE')
		`, created.ID, userID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO group_invites (group_id, token_hash, token, created_by)
			VALUES ($1, $2, $3, $4)
		`, created.ID, hash, token, userID)
		if err != nil {
			return errors.New("create invite failed")
		}
		created.MemberCount = 1
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidName) {
			return Created{}, err
		}
		return Created{}, fmt.Errorf("create group: %w", err)
	}
	created.Name = name
	created.Token = token
	return created, nil
}

// List returns the caller's active groups.
func (s *Postgres) List(ctx context.Context, userID int64) ([]Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, m.role,
			(SELECT count(*) FROM group_members mm
			  WHERE mm.group_id = g.id AND mm.status = 'ACTIVE')
		FROM group_members m
		JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND m.status = 'ACTIVE' AND g.status = 'ACTIVE'
		ORDER BY g.name, g.id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var item Summary
		if err := rows.Scan(&item.ID, &item.Name, &item.Role, &item.MemberCount); err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return out, nil
}

// Open returns the group card for an active member.
func (s *Postgres) Open(ctx context.Context, userID, groupID int64) (Card, error) {
	var card Card
	err := s.pool.QueryRow(ctx, `
		SELECT g.id, g.name, m.role,
			COALESCE((SELECT u.display_name
			   FROM group_members om
			   JOIN users u ON u.id = om.user_id
			  WHERE om.group_id = g.id AND om.role = 'OWNER' AND om.status = 'ACTIVE'
			  LIMIT 1), ''),
			(SELECT count(*) FROM group_members mm
			  WHERE mm.group_id = g.id AND mm.status = 'ACTIVE')
		FROM group_members m
		JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND m.group_id = $2
		  AND m.status = 'ACTIVE' AND g.status = 'ACTIVE'
	`, userID, groupID).Scan(&card.ID, &card.Name, &card.Role, &card.OwnerName, &card.MemberCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Card{}, ErrNotMember
	}
	if err != nil {
		return Card{}, fmt.Errorf("open group: %w", err)
	}
	var token *string
	err = s.pool.QueryRow(ctx, `
		SELECT token
		FROM group_invites
		WHERE group_id = $1
		  AND token IS NOT NULL AND token <> ''
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		  AND (max_uses IS NULL OR used_count < max_uses)
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, groupID).Scan(&token)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Card{}, fmt.Errorf("open group: %w", err)
	}
	if token != nil {
		card.Token = *token
	}
	return card, nil
}

// Members lists active members for the owner. Everyone else gets only the owner.
func (s *Postgres) Members(ctx context.Context, userID, groupID int64) ([]Person, error) {
	card, err := s.Open(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT u.display_name, m.role
		FROM group_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.group_id = $1 AND m.status = 'ACTIVE'
	`
	if card.Role != "OWNER" {
		query += ` AND m.role = 'OWNER'`
	}
	query += `
		ORDER BY CASE m.role WHEN 'OWNER' THEN 0 WHEN 'ADMIN' THEN 1 ELSE 2 END,
		         u.display_name, u.id
	`
	rows, err := s.pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.DisplayName, &person.Role); err != nil {
			return nil, fmt.Errorf("list members: %w", err)
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return out, nil
}

// Preview checks an invite without joining or consuming it.
func (s *Postgres) Preview(ctx context.Context, token string) (Preview, error) {
	var preview Preview
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, g.id, g.name
		FROM group_invites i
		JOIN groups g ON g.id = i.group_id
		WHERE i.token_hash = $1
		  AND g.status = 'ACTIVE'
		  AND i.revoked_at IS NULL
		  AND (i.expires_at IS NULL OR i.expires_at > now())
		  AND (i.max_uses IS NULL OR i.used_count < i.max_uses)
	`, hashToken(token)).Scan(&preview.InviteID, &preview.GroupID, &preview.GroupName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preview{}, ErrInviteInvalid
	}
	if err != nil {
		return Preview{}, fmt.Errorf("preview invite: %w", err)
	}
	return preview, nil
}

// Accept adds the person to the invite's group. A second accept does not consume the invite again.
func (s *Postgres) Accept(ctx context.Context, userID, inviteID int64) (Join, error) {
	var joined Join
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		var revoked *time.Time
		var expires *time.Time
		var maxUses *int
		var used int
		err := tx.QueryRow(ctx, `
			SELECT g.id, g.name, g.status, i.revoked_at, i.expires_at, i.max_uses, i.used_count
			FROM group_invites i
			JOIN groups g ON g.id = i.group_id
			WHERE i.id = $1
			FOR UPDATE OF i, g
		`, inviteID).Scan(&joined.GroupID, &joined.GroupName, &status, &revoked, &expires, &maxUses, &used)
		if errors.Is(err, pgx.ErrNoRows) || !inviteUsable(status, revoked, expires, maxUses, used) {
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			return ErrInviteInvalid
		}
		if err != nil {
			return err
		}

		var memberStatus string
		memberErr := tx.QueryRow(ctx, `
			SELECT status FROM group_members
			WHERE group_id = $1 AND user_id = $2
			FOR UPDATE
		`, joined.GroupID, userID).Scan(&memberStatus)
		if memberErr != nil && !errors.Is(memberErr, pgx.ErrNoRows) {
			return memberErr
		}
		if memberErr == nil && memberStatus == "ACTIVE" {
			joined.AlreadyMember = true
		} else {
			tag, err := tx.Exec(ctx, `
				UPDATE group_invites
				SET used_count = used_count + 1
				WHERE id = $1
				  AND revoked_at IS NULL
				  AND (expires_at IS NULL OR expires_at > now())
				  AND (max_uses IS NULL OR used_count < max_uses)
			`, inviteID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrInviteInvalid
			}
			if errors.Is(memberErr, pgx.ErrNoRows) {
				_, err = tx.Exec(ctx, `
					INSERT INTO group_members (group_id, user_id, role, status)
					VALUES ($1, $2, 'MEMBER', 'ACTIVE')
				`, joined.GroupID, userID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE group_members
					SET status = 'ACTIVE', left_at = NULL, joined_at = now()
					WHERE group_id = $1 AND user_id = $2
				`, joined.GroupID, userID)
			}
			if err != nil {
				return err
			}
			gameID, gameName, err := games.EnrollNewMember(ctx, tx, joined.GroupID, userID)
			if err != nil {
				return err
			}
			joined.GameID = gameID
			joined.GameName = gameName
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM group_members
			WHERE group_id = $1 AND status = 'ACTIVE'
		`, joined.GroupID).Scan(&joined.MemberCount)
	})
	if err != nil {
		if errors.Is(err, ErrInviteInvalid) {
			return Join{}, err
		}
		return Join{}, fmt.Errorf("accept invite: %w", err)
	}
	return joined, nil
}

func inviteUsable(status string, revoked, expires *time.Time, maxUses *int, used int) bool {
	if status != "ACTIVE" || revoked != nil {
		return false
	}
	if expires != nil && !expires.After(time.Now()) {
		return false
	}
	if maxUses != nil && used >= *maxUses {
		return false
	}
	return true
}
