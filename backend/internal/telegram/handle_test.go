package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/users"
)

func newH(store *memStore, directory groups.Directory, send *memSend) *Handler {
	if directory == nil {
		directory = &memGroups{}
	}
	return NewHandler(store, directory, send, nil, Options{BotUsername: "secret_santa_bot", SessionTTL: time.Hour})
}

type memStore struct {
	claimed  map[int64]bool
	users    map[int64]users.Identity
	offset   int64
	sessions map[int64]string
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

func (m *memStore) Session(_ context.Context, userID int64, _ time.Duration) (string, error) {
	return m.sessions[userID], nil
}

func (m *memStore) SaveSession(_ context.Context, userID int64, state string) error {
	if m.sessions == nil {
		m.sessions = map[int64]string{}
	}
	m.sessions[userID] = state
	return nil
}

func (m *memStore) ClearSession(_ context.Context, userID int64) error {
	delete(m.sessions, userID)
	return nil
}

type memGroups struct {
	list       []groups.Summary
	created    []string
	preview    groups.Preview
	previewErr error
	joins      int
	joined     bool
}

func (m *memGroups) Create(_ context.Context, _ int64, name string) (groups.Created, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return groups.Created{}, groups.ErrInvalidName
	}
	m.created = append(m.created, name)
	return groups.Created{ID: int64(len(m.created)), Name: name, Token: "aaaaaaaaaaaaaaaaaaaaaa", MemberCount: 1}, nil
}

func (m *memGroups) List(context.Context, int64) ([]groups.Summary, error) {
	return m.list, nil
}

func (m *memGroups) Open(context.Context, int64, int64) (groups.Card, error) {
	if len(m.list) == 0 {
		return groups.Card{}, groups.ErrNotMember
	}
	item := m.list[0]
	return groups.Card{ID: item.ID, Name: item.Name, Role: item.Role, MemberCount: item.MemberCount, Token: "aaaaaaaaaaaaaaaaaaaaaa"}, nil
}

func (m *memGroups) Members(context.Context, int64, int64) ([]groups.Person, error) {
	return []groups.Person{{DisplayName: "Анна", Role: "OWNER"}}, nil
}

func (m *memGroups) Preview(context.Context, string) (groups.Preview, error) {
	if m.previewErr != nil {
		return groups.Preview{}, m.previewErr
	}
	if m.preview.InviteID == 0 {
		return groups.Preview{}, groups.ErrInviteInvalid
	}
	return m.preview, nil
}

func (m *memGroups) Accept(context.Context, int64, int64) (groups.Join, error) {
	m.joins++
	return groups.Join{GroupID: m.preview.GroupID, GroupName: m.preview.GroupName, MemberCount: 2, AlreadyMember: m.joined}, nil
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
	h := newH(store, nil, send)
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
	h := newH(store, nil, send)
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
	h := newH(&memStore{}, nil, send)
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
	directory := &memGroups{preview: groups.Preview{InviteID: 4, GroupID: 9, GroupName: "Друзья"}}
	h := newH(&memStore{}, directory, send)
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
	if !strings.Contains(send.msgs[0].text, "Друзья") || !strings.Contains(strings.Join(send.msgs[0].data, ","), "Вступить|inv:accept:4") {
		t.Fatalf("preview %+v", send.msgs[0])
	}
	if directory.joins != 0 {
		t.Fatal("preview joined")
	}
}

func TestGroupChatIsIgnored(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	h := newH(store, nil, send)
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
	store := &memStore{}
	send := &memSend{}
	directory := &memGroups{list: []groups.Summary{{ID: 3, Name: "Друзья", Role: "OWNER", MemberCount: 1}}}
	h := newH(store, directory, send)
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
	if len(edited) != 3 || !strings.Contains(edited[0], "Друзья") || edited[1] != askGroupNameText || edited[2] != unavailableText {
		t.Fatalf("edits %v", edited)
	}
	if len(toasts) != 3 || toasts[2] != unavailableText {
		t.Fatalf("toasts %v", toasts)
	}
}

func TestCreateGroupFromName(t *testing.T) {
	store := &memStore{sessions: map[int64]string{1: "group_name"}}
	send := &memSend{}
	directory := &memGroups{}
	h := newH(store, directory, send)
	err := h.Handle(context.Background(), Update{
		UpdateID: 11,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "  Друзья ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(directory.created) != 1 || directory.created[0] != "Друзья" {
		t.Fatalf("created %v", directory.created)
	}
	if store.sessions[1] != "" {
		t.Fatal("session was not cleared")
	}
	if !strings.Contains(send.msgs[0].text, "https://t.me/secret_santa_bot?start=join_aaaaaaaaaaaaaaaaaaaaaa") || !strings.Contains(send.msgs[0].text, "владелец") {
		t.Fatalf("card %q", send.msgs[0].text)
	}
}

func TestCancelClearsGroupWizard(t *testing.T) {
	store := &memStore{sessions: map[int64]string{1: "group_name"}}
	directory := &memGroups{}
	h := newH(store, directory, &memSend{})
	err := h.Handle(context.Background(), Update{
		UpdateID: 12,
		Message: &Message{
			From: &User{ID: 1, FirstName: "A"},
			Chat: Chat{ID: 1, Type: "private"},
			Text: "Отмена",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.sessions[1] != "" || len(directory.created) != 0 {
		t.Fatalf("session %q created %v", store.sessions[1], directory.created)
	}
}

func TestAcceptInviteJoins(t *testing.T) {
	directory := &memGroups{preview: groups.Preview{InviteID: 4, GroupID: 9, GroupName: "Друзья"}}
	send := &memSend{}
	h := newH(&memStore{}, directory, send)
	from := User{ID: 2, FirstName: "Борис"}
	msg := &Message{MessageID: 8, Chat: Chat{ID: 2, Type: "private"}, From: &from}
	err := h.Handle(context.Background(), Update{
		UpdateID:      13,
		CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "inv:accept:4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if directory.joins != 1 {
		t.Fatalf("joins %d", directory.joins)
	}
	var edited string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
		}
	}
	if !strings.Contains(edited, "Ты присоединился к группе «Друзья»!") {
		t.Fatalf("edited %q", edited)
	}
}
