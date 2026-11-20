package telegram

import (
	"context"
	"errors"
	"fmt"

	"github.com/alekseymakarkin/secret-santa/internal/draws"
	"github.com/alekseymakarkin/secret-santa/internal/games"
)

func (h *Handler) drawScreen(ctx context.Context, userID, gameID int64) (string, Keyboard, string) {
	view, err := h.games.Open(ctx, userID, gameID)
	if text, keyboard, ok := h.openGameFailure(err); ok {
		return text, keyboard, ""
	}
	if view.Role != "OWNER" && view.Role != "ADMIN" {
		return forbiddenText, mainKeyboard(), ""
	}
	if view.Status == "DRAWN" {
		return alreadyDrawnText, mainKeyboard(), ""
	}
	if view.Status != "REGISTRATION" && view.Status != "LOCKED" {
		return unavailableText, mainKeyboard(), ""
	}
	text := fmt.Sprintf("Начать жеребьёвку в «%s»?\nКаждый, кто подтвердил участие, получит имя своего получателя в личном сообщении.", view.Name)
	keyboard := Keyboard{Rows: [][]Button{
		{{Text: "Да, начать жеребьёвку", Data: fmt.Sprintf("game:draw_ok:%d", view.ID)}},
		{{Text: "К игре", Data: fmt.Sprintf("game:open:%d", view.ID)}},
	}}
	return text, keyboard, ""
}

func (h *Handler) drawConfirm(ctx context.Context, userID, gameID int64) (string, Keyboard, string) {
	err := h.draws.Run(ctx, userID, gameID)
	if errors.Is(err, draws.ErrTooFewParticipants) {
		return tooFewText, mainKeyboard(), ""
	}
	if errors.Is(err, draws.ErrAlreadyDrawn) {
		return alreadyDrawnText, mainKeyboard(), ""
	}
	if errors.Is(err, draws.ErrForbidden) {
		return forbiddenText, mainKeyboard(), ""
	}
	if errors.Is(err, draws.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("draw", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	h.log.Info("draw created", "game_id", gameID)
	view, err := h.games.Open(ctx, userID, gameID)
	if err != nil {
		return drawDoneText, mainKeyboard(), ""
	}
	text, keyboard := h.showGame(view, drawDoneText)
	return text, keyboard, ""
}

func (h *Handler) mineScreen(ctx context.Context, userID, gameID int64) (string, Keyboard, string) {
	mine, err := h.draws.ViewMine(ctx, userID, gameID)
	if errors.Is(err, draws.ErrNotMember) {
		return notMemberText, mainKeyboard(), ""
	}
	if errors.Is(err, draws.ErrNotAssigned) {
		return notAssignedText, mainKeyboard(), ""
	}
	if err != nil {
		h.log.Error("view assignment", "game_id", gameID, "err", err.Error())
		return unavailableText, mainKeyboard(), ""
	}
	text := draws.FormatAssignment(mine.GameName, mine.ReceiverName, mine.Description, mine.Exchange)
	keyboard := Keyboard{Rows: [][]Button{
		{{Text: "К игре", Data: fmt.Sprintf("game:open:%d", mine.GameID)}},
	}}
	return text, keyboard, ""
}

func (h *Handler) openGameFailure(err error) (string, Keyboard, bool) {
	if err == nil {
		return "", Keyboard{}, false
	}
	if errors.Is(err, games.ErrNotMember) {
		return notMemberText, mainKeyboard(), true
	}
	if errors.Is(err, games.ErrNotInGame) {
		return notInGameText, mainKeyboard(), true
	}
	h.log.Error("open game", "err", err.Error())
	return unavailableText, mainKeyboard(), true
}
