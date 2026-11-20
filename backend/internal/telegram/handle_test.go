package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/draws"
	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/users"
)

func newH(store *memStore, directory groups.Directory, send *memSend) *Handler {
	return newHWithGames(store, directory, &memGames{}, send)
}

func newHWithGames(store *memStore, directory groups.Directory, catalog games.Catalog, send *memSend) *Handler {
	if directory == nil {
		directory = &memGroups{}
	}
	if catalog == nil {
		catalog = &memGames{}
	}
	return NewHandler(store, directory, catalog, &memDraws{}, send, nil, Options{BotUsername: "secret_santa_bot", SessionTTL: time.Hour})
}

func newHWithDraws(store *memStore, directory groups.Directory, catalog games.Catalog, runner draws.Runner, send *memSend) *Handler {
	if directory == nil {
		directory = &memGroups{}
	}
	if catalog == nil {
		catalog = &memGames{}
	}
	if runner == nil {
		runner = &memDraws{}
	}
	return NewHandler(store, directory, catalog, runner, send, nil, Options{BotUsername: "secret_santa_bot", SessionTTL: time.Hour})
}

type memStore struct {
	claimed  map[int64]bool
	users    map[int64]users.Identity
	offset   int64
	sessions map[int64]string
	contexts map[int64][]byte
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

func (m *memStore) Session(_ context.Context, userID int64, _ time.Duration) (string, []byte, error) {
	return m.sessions[userID], m.contexts[userID], nil
}

func (m *memStore) SaveSession(_ context.Context, userID int64, state string, raw []byte) error {
	if m.sessions == nil {
		m.sessions = map[int64]string{}
	}
	m.sessions[userID] = state
	if m.contexts == nil {
		m.contexts = map[int64][]byte{}
	}
	m.contexts[userID] = raw
	return nil
}

func (m *memStore) ClearSession(_ context.Context, userID int64) error {
	delete(m.sessions, userID)
	delete(m.contexts, userID)
	return nil
}

type memGames struct {
	current   games.View
	has       bool
	created   games.Input
	calls     int
	list      []games.Summary
	people    []games.Participant
	peopleErr error
	invite    games.InviteResult
	inviteErr error
}

func (m *memGames) Create(_ context.Context, _ int64, groupID int64, input games.Input) (games.View, error) {
	m.calls++
	m.created = input
	return games.View{
		ID: 5, GroupID: groupID, Name: input.Name, Status: "REGISTRATION",
		Description: input.Description, Deadline: input.Deadline, Exchange: input.Exchange,
		Participants: 1, Mine: games.Pending,
	}, nil
}

func (m *memGames) Current(context.Context, int64, int64) (games.View, bool, error) {
	return m.current, m.has, nil
}

func (m *memGames) List(context.Context, int64) ([]games.Summary, error) {
	return m.list, nil
}

func (m *memGames) AcceptInvite(context.Context, int64, string) (games.InviteResult, error) {
	if m.inviteErr != nil {
		return games.InviteResult{}, m.inviteErr
	}
	return m.invite, nil
}

func (m *memGames) Open(context.Context, int64, int64) (games.View, error) {
	if !m.has {
		return games.View{}, games.ErrNotInGame
	}
	return m.current, nil
}

func (m *memGames) Decide(context.Context, int64, int64, string) (games.View, error) {
	return m.current, nil
}

type memDraws struct {
	err  error
	runs int
	mine draws.Mine
}

func (m *memDraws) Run(context.Context, int64, int64) error {
	m.runs++
	return m.err
}

func (m *memDraws) ViewMine(context.Context, int64, int64) (draws.Mine, error) {
	if m.err != nil {
		return draws.Mine{}, m.err
	}
	return m.mine, nil
}

func (m *memGames) People(context.Context, int64, int64) ([]games.Participant, error) {
	if m.peopleErr != nil {
		return nil, m.peopleErr
	}
	return m.people, nil
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
	want := []string{"Мои группы|grp:list", "Мои игры|game:list", "Создать группу|grp:new", "Помощь|menu:help"}
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

func TestCreateGameWizard(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	catalog := &memGames{}
	directory := &memGroups{list: []groups.Summary{{ID: 3, Name: "Друзья", Role: "OWNER", MemberCount: 2}}}
	h := newHWithGames(store, directory, catalog, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	private := func(updateID int64, text string) Update {
		return Update{UpdateID: updateID, Message: &Message{From: &from, Chat: Chat{ID: 7, Type: "private"}, Text: text}}
	}
	steps := []Update{
		{UpdateID: 20, CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:create:3"}},
		private(21, "  Тайный Санта 2026 "),
		private(22, "  до 2000 ₽, красный "),
		private(23, "01.01.2030"),
		private(24, "-"),
	}
	for _, upd := range steps {
		if err := h.Handle(context.Background(), upd); err != nil {
			t.Fatal(err)
		}
	}
	if catalog.calls != 1 || catalog.created.Name != "Тайный Санта 2026" || catalog.created.Description != "до 2000 ₽, красный" || catalog.created.Exchange != "" {
		t.Fatalf("created %+v calls %d", catalog.created, catalog.calls)
	}
	day, _, err := games.ParseDay("01.01.2030")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.created.Deadline == nil || !catalog.created.Deadline.Equal(games.EndOfDay(day)) {
		t.Fatalf("deadline %v", catalog.created.Deadline)
	}
	if store.sessions[7] != "" {
		t.Fatal("session left open")
	}
	var asked bool
	for _, m := range send.msgs {
		if strings.Contains(m.text, "Можешь написать здесь пожелания к подаркам.") {
			asked = true
		}
	}
	if !asked {
		t.Fatal("description hint was not shown")
	}
	last := send.msgs[len(send.msgs)-1].text
	for _, part := range []string{"Игра создана.", "Регистрация", "Описание: до 2000 ₽, красный", "Регистрация до: 01.01.2030", "Ты ещё не подтвердил участие."} {
		if !strings.Contains(last, part) {
			t.Fatalf("card %q missing %s", last, part)
		}
	}
}

func TestMemberCannotCreateGame(t *testing.T) {
	store := &memStore{}
	send := &memSend{}
	directory := &memGroups{list: []groups.Summary{{ID: 3, Name: "Друзья", Role: "MEMBER", MemberCount: 2}}}
	h := newH(store, directory, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	err := h.Handle(context.Background(), Update{
		UpdateID:      30,
		CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:create:3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var edited string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
		}
	}
	if edited != forbiddenText || store.sessions[7] != "" {
		t.Fatalf("edited %q session %q", edited, store.sessions[7])
	}
}

func TestOwnerSeesParticipantsByStatus(t *testing.T) {
	send := &memSend{}
	catalog := &memGames{
		has: true,
		current: games.View{
			ID: 5, GroupID: 3, Name: "Тайный Санта 2026", Role: "OWNER", Status: "REGISTRATION",
		},
		people: []games.Participant{
			{DisplayName: "Анна", Status: games.Confirmed, AssignmentState: "NOT_APPLICABLE"},
			{DisplayName: "Борис", Status: games.Declined, AssignmentState: "NOT_APPLICABLE"},
			{DisplayName: "Виктор", Status: games.Confirmed, AssignmentState: "AWAITING_ASSIGNMENT"},
			{DisplayName: "Глеб", Status: games.Pending, AssignmentState: "NOT_APPLICABLE"},
		},
	}
	h := newHWithGames(&memStore{}, &memGroups{}, catalog, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	err := h.Handle(context.Background(), Update{
		UpdateID:      40,
		CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:people:5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var edited string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
		}
	}
	for _, part := range []string{
		"Подтвердили:\n• Анна",
		"Ожидают решения:\n• Глеб",
		"Отказались:\n• Борис",
		"Ожидают включения:\n• Виктор",
	} {
		if !strings.Contains(edited, part) {
			t.Fatalf("list %q missing %s", edited, part)
		}
	}
	if strings.Contains(edited, "Подтвердили:\n• Виктор") {
		t.Fatalf("waiting person listed as confirmed: %q", edited)
	}
}

func TestGameInviteConfirmsOnStart(t *testing.T) {
	send := &memSend{}
	directory := &memGroups{}
	catalog := &memGames{invite: games.InviteResult{
		View:      games.View{ID: 5, GroupID: 3, Name: "Тайный Санта 2026", Status: "REGISTRATION", Mine: games.Confirmed, Role: "MEMBER"},
		GroupName: "Друзья",
	}}
	h := newHWithGames(&memStore{}, directory, catalog, send)
	err := h.Handle(context.Background(), Update{
		UpdateID: 60,
		Message: &Message{
			From: &User{ID: 8, FirstName: "Борис"},
			Chat: Chat{ID: 8, Type: "private"},
			Text: "/start play_abc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if directory.joins != 0 {
		t.Fatal("group invite was consumed")
	}
	got := send.msgs[0]
	if !strings.Contains(got.text, "Ты присоединился к группе «Друзья» и участвуешь в игре «Тайный Санта 2026».") {
		t.Fatalf("text %q", got.text)
	}
	if strings.Contains(strings.Join(got.data, ","), "Подтвердить") || strings.Contains(got.text, "play_abc") {
		t.Fatalf("message %+v", got)
	}
}

func TestMyGamesOpensAGame(t *testing.T) {
	send := &memSend{}
	catalog := &memGames{list: []games.Summary{{
		ID: 5, Name: "Тайный Санта 2026", GroupName: "Друзья", Status: "REGISTRATION",
	}}}
	h := newHWithGames(&memStore{}, &memGroups{}, catalog, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	if err := h.Handle(context.Background(), Update{
		UpdateID: 42, CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:list"},
	}); err != nil {
		t.Fatal(err)
	}
	var edited string
	var data []string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
			data = m.data
		}
	}
	if !strings.Contains(edited, "Твои игры:") || !strings.Contains(edited, "Тайный Санта 2026 — Друзья") || !strings.Contains(edited, "Регистрация") {
		t.Fatalf("edited %q", edited)
	}
	if strings.Join(data, ",") != "Тайный Санта 2026|game:open:5,В меню|menu:home" {
		t.Fatalf("buttons %v", data)
	}
}

func TestMemberCannotListParticipants(t *testing.T) {
	send := &memSend{}
	catalog := &memGames{has: true, current: games.View{ID: 5, Role: "MEMBER"}, peopleErr: games.ErrForbidden}
	h := newHWithGames(&memStore{}, &memGroups{}, catalog, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	err := h.Handle(context.Background(), Update{
		UpdateID:      41,
		CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:people:5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var edited string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
		}
	}
	if edited != forbiddenText {
		t.Fatalf("edited %q", edited)
	}
}

func TestDrawAsksForConfirmationThenRuns(t *testing.T) {
	send := &memSend{}
	catalog := &memGames{has: true, current: games.View{
		ID: 5, GroupID: 3, Name: "Тайный Санта 2026", Role: "OWNER", Status: "REGISTRATION",
	}}
	runner := &memDraws{}
	h := newHWithDraws(&memStore{}, &memGroups{}, catalog, runner, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 4, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	if err := h.Handle(context.Background(), Update{
		UpdateID: 50, CallbackQuery: &CallbackQuery{ID: "cb1", From: from, Message: msg, Data: "game:draw:5"},
	}); err != nil {
		t.Fatal(err)
	}
	if runner.runs != 0 {
		t.Fatal("warning click started the draw")
	}
	catalog.current.Status = "DRAWN"
	catalog.current.Assignment = "ASSIGNED"
	if err := h.Handle(context.Background(), Update{
		UpdateID: 51, CallbackQuery: &CallbackQuery{ID: "cb2", From: from, Message: msg, Data: "game:draw_ok:5"},
	}); err != nil {
		t.Fatal(err)
	}
	if runner.runs != 1 {
		t.Fatalf("runs %d", runner.runs)
	}
	var edited []string
	for _, m := range send.msgs {
		if m.edit {
			edited = append(edited, m.text)
		}
	}
	if len(edited) != 2 || !strings.Contains(edited[0], "Начать жеребьёвку") || !strings.Contains(edited[1], drawDoneText) {
		t.Fatalf("edits %v", edited)
	}
	if strings.Contains(edited[1], "даришь подарок") {
		t.Fatalf("draw result leaked a receiver: %q", edited[1])
	}
}

func TestMineShowsOnlyTheCallerReceiver(t *testing.T) {
	send := &memSend{}
	runner := &memDraws{mine: draws.Mine{GameID: 5, GameName: "Игра", ReceiverName: "Борис", ReceiverUsername: "boris"}}
	h := newHWithDraws(&memStore{}, &memGroups{}, &memGames{}, runner, send)
	from := User{ID: 7, FirstName: "A"}
	msg := &Message{MessageID: 4, Chat: Chat{ID: 7, Type: "private"}, From: &from}
	if err := h.Handle(context.Background(), Update{
		UpdateID: 52, CallbackQuery: &CallbackQuery{ID: "cb", From: from, Message: msg, Data: "game:mine:5"},
	}); err != nil {
		t.Fatal(err)
	}
	var edited string
	for _, m := range send.msgs {
		if m.edit {
			edited = m.text
		}
	}
	if !strings.Contains(edited, "Борис (@boris)") || strings.Contains(edited, "<b>") || strings.Contains(edited, "Анна") {
		t.Fatalf("edited %q", edited)
	}
}
