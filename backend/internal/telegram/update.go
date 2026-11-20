package telegram

import "strings"

// Update is the part of a Telegram update this bot handles.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Message is a private or group message.
type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat is the conversation that carried an update.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// User is the Telegram account that sent an update.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// CallbackQuery is a button press.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// Actor is the person who sent the update. Bots are ignored.
func (u Update) Actor() (User, bool) {
	if u.CallbackQuery != nil {
		if u.CallbackQuery.From.ID == 0 || u.CallbackQuery.From.IsBot {
			return User{}, false
		}
		return u.CallbackQuery.From, true
	}
	if u.Message != nil && u.Message.From != nil && !u.Message.From.IsBot && u.Message.From.ID != 0 {
		return *u.Message.From, true
	}
	return User{}, false
}

// Private reports whether the update belongs to a private chat with the bot.
func (u Update) Private() bool {
	if u.CallbackQuery != nil {
		if u.CallbackQuery.Message == nil {
			return true
		}
		return u.CallbackQuery.Message.Chat.Type == "private"
	}
	return u.Message != nil && u.Message.Chat.Type == "private"
}

// ChatID is where a reply should be sent.
func (u Update) ChatID() int64 {
	if u.CallbackQuery != nil && u.CallbackQuery.Message != nil {
		return u.CallbackQuery.Message.Chat.ID
	}
	if u.Message != nil {
		return u.Message.Chat.ID
	}
	if u.CallbackQuery != nil {
		return u.CallbackQuery.From.ID
	}
	return 0
}

func parseCommand(text string) (name, payload string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	fields := strings.Fields(text)
	head := strings.TrimPrefix(fields[0], "/")
	if i := strings.IndexByte(head, '@'); i >= 0 {
		head = head[:i]
	}
	name = strings.ToLower(head)
	if len(fields) > 1 {
		payload = strings.Join(fields[1:], " ")
	}
	return name, payload, name != ""
}
