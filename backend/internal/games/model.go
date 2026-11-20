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
	Assignment   string
	Role         string
	InviteToken  string
}

// InviteResult is a start-link that joined the group and confirmed the game.
type InviteResult struct {
	View
	GroupName        string
	AlreadyMember    bool
	AlreadyConfirmed bool
}

// Participant is one person in a game, for the organizer's lists.
type Participant struct {
	DisplayName     string
	Username        string
	Status          string
	AssignmentState string
}

// Summary is one row in a member's game list.
type Summary struct {
	ID        int64
	Name      string
	GroupName string
	Status    string
}

// Catalog is how the bot creates a game and records a member's answer.
type Catalog interface {
	Create(ctx context.Context, userID, groupID int64, input Input) (View, error)
	Current(ctx context.Context, userID, groupID int64) (View, bool, error)
	Open(ctx context.Context, userID, gameID int64) (View, error)
	List(ctx context.Context, userID int64) ([]Summary, error)
	AcceptInvite(ctx context.Context, userID int64, token string) (InviteResult, error)
	Decide(ctx context.Context, userID, gameID int64, status string) (View, error)
	People(ctx context.Context, userID, gameID int64) ([]Participant, error)
}
