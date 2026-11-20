package games

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const maxDescriptionRunes = 1000

// Moscow is the calendar for registration and exchange dates. Russia has no daylight saving.
var Moscow = time.FixedZone("MSK", 3*60*60)

var (
	// ErrInvalidName means the game name is empty or longer than 80 characters.
	ErrInvalidName = errors.New("invalid game name")
	// ErrInvalidDescription means the gift note is empty or longer than 1000 characters.
	ErrInvalidDescription = errors.New("invalid description")
	// ErrInvalidDate means the date is not DD.MM.YYYY.
	ErrInvalidDate = errors.New("invalid date")
	// ErrForbidden means the person cannot manage this group's game.
	ErrForbidden = errors.New("forbidden")
	// ErrNotMember means the person is not an active member of the group.
	ErrNotMember = errors.New("not a member")
	// ErrNotInGame means this person has no participant row for the game.
	ErrNotInGame = errors.New("not in game")
	// ErrCurrentGameExists means the group already has an unfinished game.
	ErrCurrentGameExists = errors.New("current game exists")
	// ErrRegistrationClosed means confirmation is no longer accepted.
	ErrRegistrationClosed = errors.New("registration closed")
)

// NormalizeName trims a game name and checks its length.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 80 {
		return "", ErrInvalidName
	}
	return name, nil
}

// NormalizeDescription trims a gift note. A dash means there is no note.
func NormalizeDescription(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "-" || text == "—" || text == "–" {
		return "", nil
	}
	if text == "" || utf8.RuneCountInString(text) > maxDescriptionRunes {
		return "", ErrInvalidDescription
	}
	return text, nil
}

// ParseDay reads DD.MM.YYYY. A dash skips the date.
func ParseDay(text string) (time.Time, bool, error) {
	text = strings.TrimSpace(text)
	if text == "-" || text == "—" || text == "–" {
		return time.Time{}, true, nil
	}
	day, err := time.ParseInLocation("02.01.2006", text, Moscow)
	if err != nil || day.Year() < 2000 || day.Year() > 2100 {
		return time.Time{}, false, ErrInvalidDate
	}
	return day, false, nil
}

// EndOfDay is the exclusive end of that Moscow calendar day.
func EndOfDay(day time.Time) time.Time {
	year, month, date := day.In(Moscow).Date()
	return time.Date(year, month, date, 0, 0, 0, 0, Moscow).AddDate(0, 0, 1)
}
