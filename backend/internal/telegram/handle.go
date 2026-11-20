package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/draws"
	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/users"
)

var joinTokenLog = regexp.MustCompile(`(?:join|play)_[A-Za-z0-9_-]+`)

// Store is the persistence for one polling process.
type Store interface {
	Claim(ctx context.Context, updateID int64, id users.Identity, present, markDM bool) (claimed bool, userID int64, err error)
	SaveOffset(ctx context.Context, next int64) error
	Offset(ctx context.Context) (int64, error)
	Session(ctx context.Context, userID int64, ttl time.Duration) (string, []byte, error)
	SaveSession(ctx context.Context, userID int64, state string, raw []byte) error
	ClearSession(ctx context.Context, userID int64) error
}

// Sender delivers menu screens to Telegram.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string, keyboard Keyboard) error
	EditMessageText(ctx context.Context, chatID int64, messageID int, text string, keyboard Keyboard) error
	AnswerCallbackQuery(ctx context.Context, callbackID, text string) error
}

// Options configures the private-chat handler.
type Options struct {
	BotUsername string
	SessionTTL  time.Duration
}

// Handler turns private updates into the group and invite flows.
type Handler struct {
	store    Store
	groups   groups.Directory
	games    games.Catalog
	draws    draws.Runner
	send     Sender
	log      *slog.Logger
	username string
	ttl      time.Duration
}

// NewHandler builds the update handler.
func NewHandler(store Store, directory groups.Directory, catalog games.Catalog, runner draws.Runner, send Sender, log *slog.Logger, opts Options) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 30 * time.Minute
	}
	return &Handler{
		store:    store,
		groups:   directory,
		games:    catalog,
		draws:    runner,
		send:     send,
		log:      log,
		username: opts.BotUsername,
		ttl:      opts.SessionTTL,
	}
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
		h.log.Error("telegram reply", "update_id", upd.UpdateID, "err", joinTokenLog.ReplaceAllString(err.Error(), "invite_***"))
	}
	return nil
}

func (h *Handler) reply(ctx context.Context, upd Update, userID int64) error {
	if upd.CallbackQuery != nil {
		if err := h.store.ClearSession(ctx, userID); err != nil {
			return err
		}
		return h.callback(ctx, upd, userID)
	}
	text := ""
	if upd.Message != nil {
		text = upd.Message.Text
	}
	name, payload, isCommand := parseCommand(text)
	if isCommand {
		if err := h.store.ClearSession(ctx, userID); err != nil {
			return err
		}
		switch name {
		case "start":
			text, keyboard := h.startScreen(ctx, userID, payload)
			return h.send.SendMessage(ctx, upd.ChatID(), text, keyboard)
		case "help":
			return h.send.SendMessage(ctx, upd.ChatID(), helpText, mainKeyboard())
		default:
			return h.send.SendMessage(ctx, upd.ChatID(), menuText, mainKeyboard())
		}
	}
	if strings.EqualFold(strings.TrimSpace(text), "Отмена") {
		if err := h.store.ClearSession(ctx, userID); err != nil {
			return err
		}
		return h.send.SendMessage(ctx, upd.ChatID(), menuText, mainKeyboard())
	}
	state, raw, err := h.store.Session(ctx, userID, h.ttl)
	if err != nil {
		return err
	}
	switch state {
	case "group_name":
		return h.createGroup(ctx, upd.ChatID(), userID, text)
	case "game_name", "game_description", "game_deadline", "game_exchange":
		return h.gameWizard(ctx, upd.ChatID(), userID, state, raw, text)
	default:
		return h.send.SendMessage(ctx, upd.ChatID(), menuText, mainKeyboard())
	}
}

func (h *Handler) startScreen(ctx context.Context, userID int64, payload string) (string, Keyboard) {
	if payload == "" {
		return menuText, mainKeyboard()
	}
	if token, ok := joinToken(payload); ok {
		preview, err := h.groups.Preview(ctx, token)
		if err != nil {
			h.log.Info("invite preview rejected", "err", err.Error())
			return inviteInvalidText, mainKeyboard()
		}
		h.log.Info("invite preview", "invite_id", preview.InviteID, "group_id", preview.GroupID)
		return joinPreviewScreen(preview)
	}
	if token, ok := playToken(payload); ok {
		return h.acceptGameInvite(ctx, userID, token)
	}
	return invalidLinkText, mainKeyboard()
}

func (h *Handler) acceptGameInvite(ctx context.Context, userID int64, token string) (string, Keyboard) {
	result, err := h.games.AcceptInvite(ctx, userID, token)
	if errors.Is(err, games.ErrRegistrationClosed) {
		return registrationClosedText, mainKeyboard()
	}
	if err != nil {
		if !errors.Is(err, games.ErrInviteInvalid) {
			h.log.Error("game invite", "err", err.Error())
		}
		return inviteInvalidText, mainKeyboard()
	}
	h.log.Info("game invite accepted", "game_id", result.ID, "group_id", result.GroupID, "already", result.AlreadyMember)
	intro := fmt.Sprintf("🎄 Ты участвуешь в игре «%s».", result.Name)
	if !result.AlreadyMember {
		intro = fmt.Sprintf("🎄 Ты присоединился к группе «%s» и участвуешь в игре «%s».", result.GroupName, result.Name)
	} else if result.AlreadyConfirmed {
		intro = fmt.Sprintf("Ты уже участвуешь в игре «%s».", result.Name)
	}
	return h.showGame(result.View, intro)
}

func (h *Handler) createGroup(ctx context.Context, chatID, userID int64, name string) error {
	created, err := h.groups.Create(ctx, userID, name)
	if errors.Is(err, groups.ErrInvalidName) {
		return h.send.SendMessage(ctx, chatID, invalidNameText+"\n\n"+askGroupNameText, Keyboard{})
	}
	if err != nil {
		h.log.Error("create group", "err", err.Error())
		return h.send.SendMessage(ctx, chatID, createFailedText, Keyboard{})
	}
	if err := h.store.ClearSession(ctx, userID); err != nil {
		return err
	}
	h.log.Info("group created", "group_id", created.ID)
	text, keyboard := groupCardScreen(groups.Card{
		ID:          created.ID,
		Name:        created.Name,
		Role:        "OWNER",
		MemberCount: created.MemberCount,
		Token:       created.Token,
	}, deeplink(h.username, created.Token), nil)
	return h.send.SendMessage(ctx, chatID, text, keyboard)
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

func (h *Handler) callbackScreen(ctx context.Context, data string, userID int64) (string, Keyboard, string) {
	verb, args, ok := parseCallback(data)
	if !ok {
		return unavailableText, mainKeyboard(), unavailableText
	}
	switch verb {
	case "menu:home":
		return menuText, mainKeyboard(), ""
	case "menu:help":
		return helpText, mainKeyboard(), ""
	case "grp:new":
		if err := h.store.SaveSession(ctx, userID, "group_name", nil); err != nil {
			h.log.Error("save session", "err", err.Error())
			return createFailedText, mainKeyboard(), ""
		}
		return askGroupNameText, Keyboard{}, ""
	case "grp:list":
		return h.listScreen(ctx, userID)
	case "game:list":
		return h.gamesScreen(ctx, userID)
	case "grp:open":
		return h.openScreen(ctx, userID, args[0])
	case "grp:members":
		return h.membersScreen(ctx, userID, args[0])
	case "inv:accept":
		return h.acceptScreen(ctx, userID, args[0])
	case "game:create":
		return h.startGame(ctx, userID, args[0])
	case "game:open":
		return h.gameScreen(ctx, userID, args[0])
	case "game:confirm":
		return h.decideGame(ctx, userID, args[0], games.Confirmed)
	case "game:decline":
		return h.decideGame(ctx, userID, args[0], games.Declined)
	case "game:people":
		return h.peopleScreen(ctx, userID, args[0])
	case "game:draw":
		return h.drawScreen(ctx, userID, args[0])
	case "game:draw_ok":
		return h.drawConfirm(ctx, userID, args[0])
	case "game:mine":
		return h.mineScreen(ctx, userID, args[0])
	default:
		return unavailableText, mainKeyboard(), unavailableText
	}
}

func (h *Handler) listScreen(ctx context.Context, userID int64) (string, Keyboard, string) {
	items, err := h.groups.List(ctx, userID)
	if err != nil {
		h.log.Error("list groups", "err", err.Error())
		return noGroupsText, mainKeyboard(), ""
	}
	text, keyboard := groupListScreen(items)
	return text, keyboard, ""
}

func (h *Handler) gamesScreen(ctx context.Context, userID int64) (string, Keyboard, string) {
	items, err := h.games.List(ctx, userID)
	if err != nil {
		h.log.Error("list games", "err", err.Error())
		return noGamesText, mainKeyboard(), ""
	}
	text, keyboard := gameListScreen(items)
	return text, keyboard, ""
}

func (h *Handler) openScreen(ctx context.Context, userID, groupID int64) (string, Keyboard, string) {
	card, err := h.groups.Open(ctx, userID, groupID)
	if errors.Is(err, groups.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("open group", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	current, ok, err := h.games.Current(ctx, userID, groupID)
	if err != nil {
		h.log.Error("current game", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	var game *games.View
	if ok {
		game = &current
	}
	text, keyboard := groupCardScreen(card, deeplink(h.username, card.Token), game)
	return text, keyboard, ""
}

func (h *Handler) membersScreen(ctx context.Context, userID, groupID int64) (string, Keyboard, string) {
	card, err := h.groups.Open(ctx, userID, groupID)
	if errors.Is(err, groups.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("open group", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	people, err := h.groups.Members(ctx, userID, groupID)
	if err != nil {
		h.log.Error("list members", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	text, keyboard := membersScreen(card, people)
	return text, keyboard, ""
}

func (h *Handler) acceptScreen(ctx context.Context, userID, inviteID int64) (string, Keyboard, string) {
	result, err := h.groups.Accept(ctx, userID, inviteID)
	if errors.Is(err, groups.ErrInviteInvalid) {
		return inviteInvalidText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("accept invite", "invite_id", inviteID, "err", err.Error())
		return inviteInvalidText, mainKeyboard(), ""
	}
	h.log.Info("invite accepted", "invite_id", inviteID, "group_id", result.GroupID, "already", result.AlreadyMember)
	text, keyboard := joinedScreen(result)
	return text, keyboard, ""
}

func playToken(payload string) (string, bool) {
	if !strings.HasPrefix(payload, "play_") {
		return "", false
	}
	token := strings.TrimPrefix(payload, "play_")
	if token == "" || strings.ContainsAny(token, " \t\r\n/?#") {
		return "", false
	}
	return token, true
}

func joinToken(payload string) (string, bool) {
	if !strings.HasPrefix(payload, "join_") {
		return "", false
	}
	token := strings.TrimPrefix(payload, "join_")
	if token == "" || strings.ContainsAny(token, " \t\r\n/?#") {
		return "", false
	}
	return token, true
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}
