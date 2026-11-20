package draws

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres runs a draw and reads one person's receiver.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a draw service backed by pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Runner is how the bot starts a draw and shows a private result.
type Runner interface {
	Run(ctx context.Context, userID, gameID int64) error
	ViewMine(ctx context.Context, userID, gameID int64) (Mine, error)
}

type seat struct {
	participantID int64
	userID        int64
	name          string
	username      string
}

// Run builds one cycle for the confirmed members and queues a private message for each giver.
func (s *Postgres) Run(ctx context.Context, userID, gameID int64) error {
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status, name string
		var description *string
		var exchange *string
		var groupID int64
		err := tx.QueryRow(ctx, `
			SELECT status, group_id, name, description, to_char(exchange_on, 'YYYY-MM-DD')
			FROM games
			WHERE id = $1
			FOR UPDATE
		`, gameID).Scan(&status, &groupID, &name, &description, &exchange)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotAllowed
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
		if status == "DRAWN" {
			return ErrAlreadyDrawn
		}
		if status != "REGISTRATION" && status != "LOCKED" {
			return ErrNotAllowed
		}

		people, err := confirmedSeats(ctx, tx, gameID, groupID)
		if err != nil {
			return err
		}
		if len(people) < 3 {
			return ErrTooFewParticipants
		}

		ids := make([]int64, len(people))
		byParticipant := make(map[int64]seat, len(people))
		for i, person := range people {
			ids[i] = person.participantID
			byParticipant[person.participantID] = person
		}
		edges, err := SingleCycle(ids, rand.Reader)
		if err != nil {
			return err
		}
		if err := ValidateCycle(edges); err != nil {
			return err
		}

		var version int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1 FROM draws WHERE game_id = $1
		`, gameID).Scan(&version); err != nil {
			return err
		}
		var drawID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO draws (game_id, version, status, created_by)
			VALUES ($1, $2, 'ACTIVE', $3)
			RETURNING id
		`, gameID, version, userID).Scan(&drawID); err != nil {
			return err
		}
		exchangeOn := ""
		if exchange != nil {
			exchangeOn = *exchange
		}
		note := ""
		if description != nil {
			note = *description
		}
		for _, edge := range edges {
			if _, err := tx.Exec(ctx, `
				INSERT INTO assignments (draw_id, game_id, giver_participant_id, receiver_participant_id)
				VALUES ($1, $2, $3, $4)
			`, drawID, gameID, edge.GiverID, edge.ReceiverID); err != nil {
				return err
			}
			giver := byParticipant[edge.GiverID]
			receiver := byParticipant[edge.ReceiverID]
			payload, err := json.Marshal(AssignmentPayload{
				GameName:         name,
				ReceiverName:     receiver.name,
				ReceiverUsername: receiver.username,
				Description:      note,
				ExchangeOn:       exchangeOn,
			})
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO notification_outbox (user_id, game_id, draw_id, event_type, payload, status)
				VALUES ($1, $2, $3, 'ASSIGNMENT_CREATED', $4, 'PENDING')
			`, giver.userID, gameID, drawID, payload); err != nil {
				return err
			}
		}
		participantIDs := ids
		if _, err := tx.Exec(ctx, `
			UPDATE game_participants
			SET assignment_state = 'ASSIGNED'
			WHERE id = ANY($1)
		`, participantIDs); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE games
			SET status = 'DRAWN', active_draw_id = $2, updated_at = now()
			WHERE id = $1
		`, gameID, drawID)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrTooFewParticipants) || errors.Is(err, ErrAlreadyDrawn) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrNotMember) || errors.Is(err, ErrNotAllowed) || errors.Is(err, ErrInvalidCycle) || errors.Is(err, ErrDuplicateParticipant) {
			return err
		}
		return fmt.Errorf("run draw: %w", err)
	}
	return nil
}

func confirmedSeats(ctx context.Context, tx pgx.Tx, gameID, groupID int64) ([]seat, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.user_id, u.display_name, COALESCE(u.username, '')
		FROM game_participants p
		JOIN users u ON u.id = p.user_id
		JOIN group_members m ON m.group_id = $2 AND m.user_id = p.user_id AND m.status = 'ACTIVE'
		WHERE p.game_id = $1 AND p.status = 'CONFIRMED'
		ORDER BY p.id
	`, gameID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var people []seat
	for rows.Next() {
		var person seat
		if err := rows.Scan(&person.participantID, &person.userID, &person.name, &person.username); err != nil {
			return nil, err
		}
		people = append(people, person)
	}
	return people, rows.Err()
}

// ViewMine returns only the caller's receiver from the active draw.
func (s *Postgres) ViewMine(ctx context.Context, userID, gameID int64) (Mine, error) {
	var groupID int64
	err := s.pool.QueryRow(ctx, `SELECT group_id FROM games WHERE id = $1`, gameID).Scan(&groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mine{}, ErrNotAssigned
	}
	if err != nil {
		return Mine{}, fmt.Errorf("view assignment: %w", err)
	}
	var one int
	err = s.pool.QueryRow(ctx, `
		SELECT 1
		FROM group_members m
		JOIN groups g ON g.id = m.group_id
		WHERE m.group_id = $1 AND m.user_id = $2
		  AND m.status = 'ACTIVE' AND g.status = 'ACTIVE'
	`, groupID, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mine{}, ErrNotMember
	}
	if err != nil {
		return Mine{}, fmt.Errorf("view assignment: %w", err)
	}
	var mine Mine
	var description *string
	var exchange *string
	err = s.pool.QueryRow(ctx, `
		SELECT g.id, g.group_id, g.name, g.description, to_char(g.exchange_on, 'YYYY-MM-DD'),
		       ru.display_name, COALESCE(ru.username, '')
		FROM games g
		JOIN game_participants me ON me.game_id = g.id AND me.user_id = $2
		JOIN assignments a ON a.giver_participant_id = me.id AND a.game_id = g.id
		JOIN draws d ON d.id = a.draw_id AND d.status = 'ACTIVE' AND d.id = g.active_draw_id
		JOIN game_participants them ON them.id = a.receiver_participant_id
		JOIN users ru ON ru.id = them.user_id
		WHERE g.id = $1 AND me.assignment_state = 'ASSIGNED'
	`, gameID, userID).Scan(&mine.GameID, &mine.GroupID, &mine.GameName, &description, &exchange, &mine.ReceiverName, &mine.ReceiverUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mine{}, ErrNotAssigned
	}
	if err != nil {
		return Mine{}, fmt.Errorf("view assignment: %w", err)
	}
	if description != nil {
		mine.Description = *description
	}
	if exchange != nil {
		mine.Exchange = *exchange
	}
	return mine, nil
}
