package games

import (
	"context"
	"time"
)

const (
	// Pending means an existing member has not answered yet.
	Pending = "PENDING"
	// Confirmed means the person will take part.
	Confirmed = "CONFIRMED"
	// Declined means the person refused this game.
	Declined = "DECLINED"
)

// Input is what the organizer sends to open registration.
type Input struct {
	Name        string
	Description string
	Deadline    *time.Time
	Exchange    string
}

// View is the game card for one member.
type View struct {
	ID           int64
	GroupID      int64
	Name         string
	Status       string
	Description  string
	Deadline     *time.Time
	Exchange     string
	Confirmed    int
	Participants int
	Mine         string
}

// Catalog is how the bot creates a game and records a member's answer.
type Catalog interface {
	Create(ctx context.Context, userID, groupID int64, input Input) (View, error)
	Current(ctx context.Context, userID, groupID int64) (View, bool, error)
	Open(ctx context.Context, userID, gameID int64) (View, error)
	Decide(ctx context.Context, userID, gameID int64, status string) (View, error)
}
