// Package users stores the Telegram identity of a person.
package users

import (
	"strings"
	"unicode/utf8"
)

// Identity is the Telegram profile carried by an update.
type Identity struct {
	TelegramID  int64
	Username    string
	DisplayName string
}

// User is a stored person.
type User struct {
	ID          int64
	TelegramID  int64
	Username    string
	DisplayName string
}

// Profile builds the name we store. Empty names become "Участник".
func Profile(telegramID int64, username, firstName, lastName string) Identity {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if strings.ContainsAny(username, " \t\r\n") {
		username = ""
	}
	name := strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
	if name == "" {
		name = username
	}
	if name == "" {
		name = "Участник"
	}
	if utf8.RuneCountInString(name) > 128 {
		runes := []rune(name)
		name = string(runes[:128])
	}
	return Identity{
		TelegramID:  telegramID,
		Username:    username,
		DisplayName: name,
	}
}
