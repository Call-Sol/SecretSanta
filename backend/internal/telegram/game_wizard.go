package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
)

type gameDraft struct {
	GroupID     int64  `json:"group_id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
}

func (h *Handler) startGame(ctx context.Context, userID, groupID int64) (string, Keyboard, string) {
	card, err := h.groups.Open(ctx, userID, groupID)
	if errors.Is(err, groups.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("open group", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	if card.Role != "OWNER" && card.Role != "ADMIN" {
		return forbiddenText, mainKeyboard(), ""
	}
	current, ok, err := h.games.Current(ctx, userID, groupID)
	if err != nil {
		h.log.Error("current game", "group_id", groupID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	if ok {
		text, keyboard := gameCardScreen(current, currentGameExistsText)
		return text, keyboard, ""
	}
	raw, err := json.Marshal(gameDraft{GroupID: groupID})
	if err != nil {
		return gameFailedText, mainKeyboard(), ""
	}
	if err := h.store.SaveSession(ctx, userID, "game_name", raw); err != nil {
		h.log.Error("save session", "err", err.Error())
		return gameFailedText, mainKeyboard(), ""
	}
	return askGameNameText, Keyboard{}, ""
}

func (h *Handler) gameScreen(ctx context.Context, userID, gameID int64) (string, Keyboard, string) {
	view, err := h.games.Open(ctx, userID, gameID)
	if errors.Is(err, games.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if errors.Is(err, games.ErrNotInGame) {
		return notInGameText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("open game", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	text, keyboard := gameCardScreen(view, "")
	return text, keyboard, ""
}

func (h *Handler) peopleScreen(ctx context.Context, userID, gameID int64) (string, Keyboard, string) {
	view, err := h.games.Open(ctx, userID, gameID)
	if errors.Is(err, games.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if errors.Is(err, games.ErrNotInGame) {
		return notInGameText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("open game", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	people, err := h.games.People(ctx, userID, gameID)
	if errors.Is(err, games.ErrForbidden) {
		return forbiddenText, mainKeyboard(), ""
	}
	if errors.Is(err, games.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("list participants", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	text, keyboard := gamePeopleScreen(view, people)
	return text, keyboard, ""
}

func (h *Handler) decideGame(ctx context.Context, userID, gameID int64, status string) (string, Keyboard, string) {
	view, err := h.games.Decide(ctx, userID, gameID, status)
	if errors.Is(err, games.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if errors.Is(err, games.ErrNotInGame) {
		return notInGameText, mainKeyboard(), ""
	}
	if errors.Is(err, games.ErrRegistrationClosed) {
		return registrationClosedText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("decide game", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	text, keyboard := gameCardScreen(view, "")
	return text, keyboard, ""
}

func (h *Handler) gameWizard(ctx context.Context, chatID, userID int64, state string, raw []byte, text string) error {
	var draft gameDraft
	if err := json.Unmarshal(raw, &draft); err != nil || draft.GroupID == 0 {
		if err := h.store.ClearSession(ctx, userID); err != nil {
			return err
		}
		return h.send.SendMessage(ctx, chatID, menuText, mainKeyboard())
	}
	switch state {
	case "game_name":
		name, err := games.NormalizeName(text)
		if err != nil {
			return h.send.SendMessage(ctx, chatID, invalidNameText+"\n\n"+askGameNameText, Keyboard{})
		}
		draft.Name = name
		return h.saveGameStep(ctx, chatID, userID, "game_description", draft, askGameDescriptionText)
	case "game_description":
		description, err := games.NormalizeDescription(text)
		if err != nil {
			return h.send.SendMessage(ctx, chatID, invalidDescriptionText+"\n\n"+askGameDescriptionText, Keyboard{})
		}
		draft.Description = description
		return h.saveGameStep(ctx, chatID, userID, "game_deadline", draft, askGameDeadlineText)
	case "game_deadline":
		day, skip, err := games.ParseDay(text)
		if err != nil {
			return h.send.SendMessage(ctx, chatID, invalidDateText+"\n\n"+askGameDeadlineText, Keyboard{})
		}
		if !skip && !games.EndOfDay(day).After(time.Now()) {
			return h.send.SendMessage(ctx, chatID, pastDateText+"\n\n"+askGameDeadlineText, Keyboard{})
		}
		draft.Deadline = ""
		if !skip {
			draft.Deadline = day.In(games.Moscow).Format("2006-01-02")
		}
		return h.saveGameStep(ctx, chatID, userID, "game_exchange", draft, askGameExchangeText)
	case "game_exchange":
		return h.finishGame(ctx, chatID, userID, draft, text)
	default:
		return h.send.SendMessage(ctx, chatID, menuText, mainKeyboard())
	}
}

func (h *Handler) finishGame(ctx context.Context, chatID, userID int64, draft gameDraft, text string) error {
	day, skip, err := games.ParseDay(text)
	if err != nil {
		return h.send.SendMessage(ctx, chatID, invalidDateText+"\n\n"+askGameExchangeText, Keyboard{})
	}
	input := games.Input{Name: draft.Name, Description: draft.Description}
	if draft.Deadline != "" {
		deadline, err := time.ParseInLocation("2006-01-02", draft.Deadline, games.Moscow)
		if err != nil {
			return h.send.SendMessage(ctx, chatID, invalidDateText+"\n\n"+askGameDeadlineText, Keyboard{})
		}
		end := games.EndOfDay(deadline)
		input.Deadline = &end
	}
	if !skip {
		input.Exchange = day.In(games.Moscow).Format("2006-01-02")
	}
	view, err := h.games.Create(ctx, userID, draft.GroupID, input)
	if clearErr := h.store.ClearSession(ctx, userID); clearErr != nil {
		return clearErr
	}
	if errors.Is(err, games.ErrCurrentGameExists) {
		return h.send.SendMessage(ctx, chatID, currentGameExistsText, Keyboard{Rows: [][]Button{
			{{Text: "К группе", Data: fmt.Sprintf("grp:open:%d", draft.GroupID)}},
		}})
	}
	if errors.Is(err, games.ErrForbidden) {
		return h.send.SendMessage(ctx, chatID, forbiddenText, mainKeyboard())
	}
	if errors.Is(err, games.ErrInvalidName) || errors.Is(err, games.ErrInvalidDescription) || errors.Is(err, games.ErrInvalidDate) {
		return h.send.SendMessage(ctx, chatID, gameFailedText, mainKeyboard())
	}
	if err != nil {
		h.log.Error("create game", "group_id", draft.GroupID, "err", err.Error())
		return h.send.SendMessage(ctx, chatID, gameFailedText, mainKeyboard())
	}
	h.log.Info("game created", "game_id", view.ID, "group_id", view.GroupID)
	card, keyboard := gameCardScreen(view, "Игра создана.")
	return h.send.SendMessage(ctx, chatID, card, keyboard)
}

func (h *Handler) saveGameStep(ctx context.Context, chatID, userID int64, state string, draft gameDraft, prompt string) error {
	raw, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	if err := h.store.SaveSession(ctx, userID, state, raw); err != nil {
		return err
	}
	return h.send.SendMessage(ctx, chatID, prompt, Keyboard{})
}
