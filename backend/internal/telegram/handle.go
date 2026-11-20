package telegram

import (
	"context"
	"log/slog"
	"strings"

	"github.com/alekseymakarkin/secret-santa/internal/users"
)

// Store is the persistence for one polling process.
type Store interface {
	Claim(ctx context.Context, updateID int64, id users.Identity, present, markDM bool) (claimed bool, userID int64, err error)
	SaveOffset(ctx context.Context, next int64) error
	Offset(ctx context.Context) (int64, error)
	ActiveGroupNames(ctx context.Context, userID int64) ([]string, error)
}

// Sender delivers menu screens to Telegram.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string, keyboard Keyboard) error
	EditMessageText(ctx context.Context, chatID int64, messageID int, text string, keyboard Keyboard) error
	AnswerCallbackQuery(ctx context.Context, callbackID, text string) error
}

// Handler turns a private /start into a stored user and the main menu.
type Handler struct {
	store Store
	send  Sender
	log   *slog.Logger
}

// NewHandler builds the update handler.
func NewHandler(store Store, send Sender, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{store: store, send: send, log: log}
}

// Handle claims the update, stores the person, and replies in a private chat.
// A repeated update_id does not send a second message.
func (h *Handler) Handle(ctx context.Context, upd Update) error {
	actor, ok := upd.Actor()
	private := ok && upd.Private()
	id := users.Identity{}
	if private {
		id = users.Profile(actor.ID, actor.Username, actor.FirstName, actor.LastName)
	}
	claimed, userID, err := h.store.Claim(ctx, upd.UpdateID, id, private, private)
	if err != nil {
		return err
	}
	if !claimed || !ok || !upd.Private() {
		return nil
	}
	if err := h.reply(ctx, upd, userID); err != nil {
		h.log.Error("telegram reply", "update_id", upd.UpdateID, "err", err.Error())
	}
	return nil
}

func (h *Handler) reply(ctx context.Context, upd Update, userID int64) error {
	if upd.CallbackQuery != nil {
		return h.callback(ctx, upd, userID)
	}
	text, keyboard := messageScreen(upd.Message.Text)
	return h.send.SendMessage(ctx, upd.ChatID(), text, keyboard)
}

func messageScreen(text string) (string, Keyboard) {
	name, payload, ok := parseCommand(text)
	if !ok {
		return menuText, mainKeyboard()
	}
	switch name {
	case "start":
		if payload == "" || strings.HasPrefix(payload, "join_") {
			return menuText, mainKeyboard()
		}
		return invalidLinkText, mainKeyboard()
	case "help":
		return helpText, mainKeyboard()
	default:
		return menuText, mainKeyboard()
	}
}

func (h *Handler) callback(ctx context.Context, upd Update, userID int64) error {
	text, keyboard, toast := h.callbackScreen(ctx, upd.CallbackQuery.Data, userID)
	if err := h.send.AnswerCallbackQuery(ctx, upd.CallbackQuery.ID, toast); err != nil {
		return err
	}
	if upd.CallbackQuery.Message != nil && upd.CallbackQuery.Message.MessageID != 0 {
		err := h.send.EditMessageText(ctx, upd.ChatID(), upd.CallbackQuery.Message.MessageID, text, keyboard)
		if err == nil || isNotModified(err) {
			return nil
		}
	}
	return h.send.SendMessage(ctx, upd.ChatID(), text, keyboard)
}

func (h *Handler) callbackScreen(ctx context.Context, data string, userID int64) (text string, keyboard Keyboard, toast string) {
	keyboard = mainKeyboard()
	switch data {
	case "menu:home":
		return menuText, keyboard, ""
	case "menu:help":
		return helpText, keyboard, ""
	case "grp:new":
		return createLaterText, keyboard, ""
	case "grp:list":
		names, err := h.store.ActiveGroupNames(ctx, userID)
		if err != nil {
			h.log.Error("list groups", "err", err.Error())
			return noGroupsText, keyboard, ""
		}
		return groupsText(names), keyboard, ""
	default:
		return unavailableText, keyboard, unavailableText
	}
}

func groupsText(names []string) string {
	if len(names) == 0 {
		return noGroupsText
	}
	var b strings.Builder
	b.WriteString("Твои группы:")
	for _, name := range names {
		b.WriteString("\n• ")
		b.WriteString(name)
	}
	return b.String()
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}
