package games

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres stores games and participation.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a catalog backed by pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

const viewSelect = `
	SELECT g.id, g.group_id, g.name, g.status, g.description, g.registration_deadline,
	       to_char(g.exchange_on, 'YYYY-MM-DD'),
	       (SELECT count(*) FROM game_participants cp
	         WHERE cp.game_id = g.id AND cp.status = 'CONFIRMED'),
	       (SELECT count(*) FROM game_participants ap WHERE ap.game_id = g.id),
	       COALESCE(p.status, ''),
	       m.role
	FROM games g
	JOIN group_members m ON m.group_id = g.group_id AND m.user_id = $1 AND m.status = 'ACTIVE'
	LEFT JOIN game_participants p ON p.game_id = g.id AND p.user_id = $1
`

// Create opens registration and leaves every current member unconfirmed.
func (s *Postgres) Create(ctx context.Context, userID, groupID int64, input Input) (View, error) {
	name, err := NormalizeName(input.Name)
	if err != nil {
		return View{}, err
	}
	var description string
	if input.Description != "" {
		description, err = NormalizeDescription(input.Description)
		if err != nil {
			return View{}, err
		}
	}
	if input.Exchange != "" {
		if _, err := time.Parse("2006-01-02", input.Exchange); err != nil {
			return View{}, ErrInvalidDate
		}
	}
	var gameID int64
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "ACTIVE") {
			return ErrNotMember
		}
		if err != nil {
			return err
		}
		var role string
		err = tx.QueryRow(ctx, `
			SELECT role FROM group_members
			WHERE group_id = $1 AND user_id = $2 AND status = 'ACTIVE'
		`, groupID, userID).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotMember
		}
		if err != nil {
			return err
		}
		if role != "OWNER" && role != "ADMIN" {
			return ErrForbidden
		}
		var descriptionValue any
		if description != "" {
			descriptionValue = description
		}
		var deadline any
		if input.Deadline != nil {
			deadline = *input.Deadline
		}
		var exchange any
		if input.Exchange != "" {
			exchange = input.Exchange
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO games (group_id, name, status, description, registration_deadline, exchange_on, created_by)
			VALUES ($1, $2, 'REGISTRATION', $3, $4, $5, $6)
			RETURNING id
		`, groupID, name, descriptionValue, deadline, exchange, userID).Scan(&gameID)
		if isUnique(err) {
			return ErrCurrentGameExists
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO game_participants (game_id, user_id, status, assignment_state)
			SELECT $1, user_id, 'PENDING', 'NOT_APPLICABLE'
			FROM group_members
			WHERE group_id = $2 AND status = 'ACTIVE'
		`, gameID, groupID)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrNotMember) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrCurrentGameExists) {
			return View{}, err
		}
		return View{}, fmt.Errorf("create game: %w", err)
	}
	return s.Open(ctx, userID, gameID)
}

// Current returns the unfinished game for a member, if the group has one.
func (s *Postgres) Current(ctx context.Context, userID, groupID int64) (View, bool, error) {
	if err := s.member(ctx, userID, groupID); err != nil {
		return View{}, false, err
	}
	view, err := scanView(s.pool.QueryRow(ctx, viewSelect+`
		WHERE g.group_id = $2
		  AND g.status IN ('DRAFT', 'REGISTRATION', 'LOCKED', 'DRAWN')
	`, userID, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, nil
	}
	if err != nil {
		return View{}, false, fmt.Errorf("current game: %w", err)
	}
	return view, true, nil
}

// Open returns one game for a member of its group.
func (s *Postgres) Open(ctx context.Context, userID, gameID int64) (View, error) {
	var groupID int64
	err := s.pool.QueryRow(ctx, `SELECT group_id FROM games WHERE id = $1`, gameID).Scan(&groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotInGame
	}
	if err != nil {
		return View{}, fmt.Errorf("open game: %w", err)
	}
	if err := s.member(ctx, userID, groupID); err != nil {
		return View{}, err
	}
	view, err := scanView(s.pool.QueryRow(ctx, viewSelect+` WHERE g.id = $2`, userID, gameID))
	if err != nil {
		return View{}, fmt.Errorf("open game: %w", err)
	}
	return view, nil
}

// Decide records confirm or decline while registration is open.
func (s *Postgres) Decide(ctx context.Context, userID, gameID int64, status string) (View, error) {
	if status != Confirmed && status != Declined {
		return View{}, ErrRegistrationClosed
	}
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var gameStatus string
		var deadline *time.Time
		err := tx.QueryRow(ctx, `
			SELECT g.status, g.registration_deadline
			FROM games g
			JOIN game_participants p ON p.game_id = g.id AND p.user_id = $2
			WHERE g.id = $1
			FOR UPDATE OF g, p
		`, gameID, userID).Scan(&gameStatus, &deadline)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotInGame
		}
		if err != nil {
			return err
		}
		if gameStatus != "REGISTRATION" || (deadline != nil && !deadline.After(time.Now())) {
			return ErrRegistrationClosed
		}
		_, err = tx.Exec(ctx, `
			UPDATE game_participants
			SET status = $3,
			    confirmed_at = CASE WHEN $3 = 'CONFIRMED' THEN now() ELSE NULL END
			WHERE game_id = $1 AND user_id = $2
		`, gameID, userID, status)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrNotInGame) || errors.Is(err, ErrRegistrationClosed) {
			return View{}, err
		}
		return View{}, fmt.Errorf("decide game: %w", err)
	}
	return s.Open(ctx, userID, gameID)
}

// People lists participants for the owner or an admin, grouped later by status.
func (s *Postgres) People(ctx context.Context, userID, gameID int64) ([]Participant, error) {
	view, err := s.Open(ctx, userID, gameID)
	if err != nil {
		return nil, err
	}
	if view.Role != "OWNER" && view.Role != "ADMIN" {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.display_name, p.status, p.assignment_state
		FROM game_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.game_id = $1
		ORDER BY u.display_name, u.id
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	defer rows.Close()
	var out []Participant
	for rows.Next() {
		var person Participant
		if err := rows.Scan(&person.DisplayName, &person.Status, &person.AssignmentState); err != nil {
			return nil, fmt.Errorf("list participants: %w", err)
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	return out, nil
}

// EnrollNewMember confirms a person who just joined during open registration.
// An existing participant row is left unchanged.
func EnrollNewMember(ctx context.Context, tx pgx.Tx, groupID, userID int64) (int64, string, error) {
	var gameID int64
	var name string
	err := tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO game_participants (game_id, user_id, status, assignment_state, confirmed_at)
			SELECT g.id, $2, 'CONFIRMED', 'NOT_APPLICABLE', now()
			FROM games g
			WHERE g.group_id = $1
			  AND g.status = 'REGISTRATION'
			  AND (g.registration_deadline IS NULL OR g.registration_deadline > now())
			ON CONFLICT (game_id, user_id) DO NOTHING
			RETURNING game_id
		)
		SELECT i.game_id, g.name
		FROM inserted i
		JOIN games g ON g.id = i.game_id
	`, groupID, userID).Scan(&gameID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("enroll member: %w", err)
	}
	return gameID, name, nil
}

func (s *Postgres) member(ctx context.Context, userID, groupID int64) error {
	var one int
	err := s.pool.QueryRow(ctx, `
		SELECT 1
		FROM group_members m
		JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND m.group_id = $2
		  AND m.status = 'ACTIVE' AND g.status = 'ACTIVE'
	`, userID, groupID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotMember
	}
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	return nil
}

func scanView(row pgx.Row) (View, error) {
	var view View
	var description *string
	var deadline *time.Time
	var exchange *string
	err := row.Scan(
		&view.ID, &view.GroupID, &view.Name, &view.Status, &description, &deadline, &exchange,
		&view.Confirmed, &view.Participants, &view.Mine, &view.Role,
	)
	if err != nil {
		return View{}, err
	}
	if description != nil {
		view.Description = *description
	}
	view.Deadline = deadline
	if exchange != nil {
		view.Exchange = *exchange
	}
	return view, nil
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
