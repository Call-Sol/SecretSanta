package groups

import "context"

// Created is a new group and its first invite.
type Created struct {
	ID          int64
	Name        string
	Token       string
	MemberCount int
}

// Summary is one group on the "my groups" list.
type Summary struct {
	ID          int64
	Name        string
	Role        string
	MemberCount int
}

// Card is the group screen for a member.
type Card struct {
	ID          int64
	Name        string
	Role        string
	OwnerName   string
	MemberCount int
	Token       string
}

// Person is one active member.
type Person struct {
	DisplayName string
	Role        string
}

// Preview is what the bot shows before a person joins.
type Preview struct {
	InviteID  int64
	GroupID   int64
	GroupName string
}

// Join is the result of confirming an invite.
type Join struct {
	GroupID       int64
	GroupName     string
	MemberCount   int
	AlreadyMember bool
	GameID        int64
	GameName      string
}

// Directory is how the bot creates groups and accepts invites.
type Directory interface {
	Create(ctx context.Context, userID int64, name string) (Created, error)
	List(ctx context.Context, userID int64) ([]Summary, error)
	Open(ctx context.Context, userID, groupID int64) (Card, error)
	Members(ctx context.Context, userID, groupID int64) ([]Person, error)
	Preview(ctx context.Context, token string) (Preview, error)
	Accept(ctx context.Context, userID, inviteID int64) (Join, error)
}
