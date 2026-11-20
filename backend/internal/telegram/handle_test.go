package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/alekseymakarkin/secret-santa/internal/users"
)

type memStore struct {
	claimed map[int64]bool
	users   map[int64]users.Identity
	groups  []string
	offset  int64
}

func (m *memStore) Claim(_ context.Context, updateID int64, id users.Identity, present, markDM bool) (bool, int64, error) {
	if m.claimed == nil {
		m.claimed = map[int64]bool{}
	}
	if m.claimed[updateID] {
		return false, 0, nil
	}
	m.claimed[updateID] = true
	if !present {
		return true, 0, nil
	}
	if m.users == nil {
		m.users = map[int64]users.Identity{}
	}
	if markDM {
		m.users[id.TelegramID] = id
	}
	return true, id.TelegramID, nil
}

func (m *memStore) SaveOffset(_ context.Context, next int64) error {
	m.offset = next
	return nil
}

func (m *memStore) Offset(context.Context) (int64, error) { return m.offset, nil }

func (m *memStore) ActiveGroupNames(context.Context, int64) ([]string, error) {
	return m.groups, nil
}

type sent struct {
	chat  int64
	text  string
	data  []string
	edit  bool
	toast string
}

type memSend struct {
	msgs []sent
}

func (m *memSend) SendMessage(_ context.Context, chatID int64, text string, keyboard Keyboard) error {
	m.msgs = append(m.msgs, sent{chat: chatID, text: text, data: datas(keyboard)})
	return nil
}

func (m *memSend) EditMessageText(_ context.Context, chatID int64, _ int, text string, keyboard Keyboard) error {
	m.msgs = append(m.msgs, sent{chat: chatID, text: text, data: datas(keyboard), edit: true})
	return nil
}

func (m *memSend) AnswerCallbackQuery(_ context.Context, _ string, text string) error {
	m.msgs = append(m.msgs, sent{toast: text})
	return nil
}

func datas(keyboard Keyboard) []string {
	var out []string
	for _, row := range keyboard.Rows {
		for _, button := range row {
			out = append(out, button.Text+"|"+button.Data)
		}
	}
	return out
}

func TestStartStoresUserAndShowsMenu(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	h := NewHandler(store, send, nil)
	upd := Update{
		UpdateID: 10,
		Message: &Message{
			From: &User{ID: 42, FirstName: "Анна", Username: "anna"},
			Chat: Chat{ID: 42, Type: "private"},
			Text: "/start",
		},
	}
	if err := h.Handle(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if len(send.msgs) != 1 {
		t.Fatalf("sends %d, want 1", len(send.msgs))
	}
	got := send.msgs[0]
	if got.chat != 42 || got.text != menuText {
		t.Fatalf("message %+v", got)
	}
	want := []string{"Мои группы|grp:list", "Создать группу|grp:new", "Помощь|menu:help"}
	if strings.Join(got.data, ",") != strings.Join(want, ",") {
		t.Fatalf("buttons %v", got.data)
	}
	if store.users[42].DisplayName != "Анна" {
		t.Fatalf("user %+v", store.users[42])
	}
}

func TestStartAtBotNameAndHelp(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	h := NewHandler(store, send, nil)
	err := h.Handle(context.Background(), Update{
		UpdateID: 1,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "/start@SecretSantaBot",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Handle(context.Background(), Update{
		UpdateID: 2,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "/help",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(send.msgs) != 2 || !strings.Contains(send.msgs[1].text, "Помощь") {
		t.Fatalf("messages %+v", send.msgs)
	}
}

func TestUnknownStartPayloadSaysLinkIsInvalid(t *testing.T) {
	send := &memSend{}
	h := NewHandler(&memStore{}, send, nil)
	err := h.Handle(context.Background(), Update{
		UpdateID: 3,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "/start nope",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(send.msgs[0].text, "Ссылка недействительна.") {
		t.Fatalf("text %q", send.msgs[0].text)
	}
}

func TestJoinPayloadDoesNotJoin(t *testing.T) {
	send := &memSend{}
	h := NewHandler(&memStore{}, send, nil)
	err := h.Handle(context.Background(), Update{
		UpdateID: 4,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "/start join_abc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if send.msgs[0].text != menuText {
		t.Fatalf("text %q", send.msgs[0].text)
	}
}

func TestGroupChatIsIgnored(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	h := NewHandler(store, send, nil)
	err := h.Handle(context.Background(), Update{
		UpdateID: 5,
		Message: &Message{
			From: &User{ID: 9, FirstName: "A"},
			Chat: Chat{ID: -100, Type: "supergroup"},
			Text: "/start",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(send.msgs) != 0 || len(store.users) != 0 {
		t.Fatalf("msgs %d users %d", len(send.msgs), len(store.users))
	}
	if !store.claimed[5] {
		t.Fatal("update was not claimed")
	}
}

func TestMyGroupsAndCreateButtons(t *testing.T) {
	store := &memStore{groups: []string{"Друзья"}}
	send := &memSend{}
	h := NewHandler(store, send, nil)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	if err := h.Handle(context.Background(), Update{
		UpdateID:      8,
		CallbackQuery: &CallbackQuery{ID: "cb1", From: from, Message: msg, Data: "grp:list"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), Update{
		UpdateID:      9,
		CallbackQuery: &CallbackQuery{ID: "cb2", From: from, Message: msg, Data: "grp:new"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), Update{
		UpdateID:      10,
		CallbackQuery: &CallbackQuery{ID: "cb3", From: from, Message: msg, Data: "nope"},
	}); err != nil {
		t.Fatal(err)
	}
	var edited []string
	var toasts []string
	for _, m := range send.msgs {
		if m.edit {
			edited = append(edited, m.text)
		}
		if m.toast != "" || m.text == "" {
			toasts = append(toasts, m.toast)
		}
	}
	if len(edited) != 3 || !strings.Contains(edited[0], "Друзья") || edited[1] != createLaterText || edited[2] != unavailableText {
		t.Fatalf("edits %v", edited)
	}
	if len(toasts) != 3 || toasts[2] != unavailableText {
		t.Fatalf("toasts %v", toasts)
	}
}
