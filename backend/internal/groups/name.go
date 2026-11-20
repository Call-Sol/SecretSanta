package groups

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	// ErrInvalidName means the group name is empty or longer than 80 characters.
	ErrInvalidName = errors.New("invalid group name")
	// ErrInviteInvalid means the invite is missing, revoked, expired, or used up.
	ErrInviteInvalid = errors.New("invite invalid")
	// ErrNotMember means the person is not an active member of the group.
	ErrNotMember = errors.New("not a member")
)

// NormalizeName trims a group name and checks its length.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 80 {
		return "", ErrInvalidName
	}
	return name, nil
}
