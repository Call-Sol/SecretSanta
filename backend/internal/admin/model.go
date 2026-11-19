package admin

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrArchived      = errors.New("group is archived")
	ErrInvalidName   = errors.New("invalid group name")
	ErrInvalidInvite = errors.New("invalid invite settings")
)

// Session is an authenticated operator session.
type Session struct {
	CSRF      string
	ExpiresAt time.Time
}

// Group is an internal Secret Santa group.
type Group struct {
	ID        int64
	Name      string
	Status    string
	CreatedAt time.Time
}

// GroupSummary is a group plus the counts shown on the list.
type GroupSummary struct {
	Group
	MemberCount   int
	ActiveInvites int
}

// Member is one person in a group.
type Member struct {
	DisplayName string
	Username    string
	Role        string
	Status      string
	JoinedAt    time.Time
}

// Invite is a join link without its secret token.
type Invite struct {
	ID        int64
	CreatedAt time.Time
	ExpiresAt *time.Time
	MaxUses   *int
	UsedCount int
	RevokedAt *time.Time
	State     string
	Usage     string
}

// GroupDetail is the monitoring page for one group.
type GroupDetail struct {
	Group
	Members []Member
	Invites []Invite
}

// InviteOptions are the optional limits on a new deeplink.
type InviteOptions struct {
	MaxUses   *int
	ExpiresAt *time.Time
}

// NormalizeGroupName trims a group name and checks its length.
func NormalizeGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 80 {
		return "", ErrInvalidName
	}
	return name, nil
}

func (inv *Invite) decorate(now time.Time) {
	switch {
	case inv.RevokedAt != nil:
		inv.State = "Revoked"
	case inv.ExpiresAt != nil && !inv.ExpiresAt.After(now):
		inv.State = "Expired"
	case inv.MaxUses != nil && inv.UsedCount >= *inv.MaxUses:
		inv.State = "Used up"
	default:
		inv.State = "Active"
	}
	if inv.MaxUses == nil {
		inv.Usage = fmt.Sprintf("%d / unlimited", inv.UsedCount)
		return
	}
	inv.Usage = fmt.Sprintf("%d / %d", inv.UsedCount, *inv.MaxUses)
}
