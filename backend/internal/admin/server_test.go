package admin

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
)

func TestPanelLoginGroupsAndDeeplink(t *testing.T) {
	var logs bytes.Buffer
	panel, err := New(newMemStore(), Options{
		Username:    "operator",
		Password:    "test-password",
		BotUsername: "secret_santa_bot",
		SessionTTL:  time.Hour,
		Log:         slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(panel.Handler())
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginPage := clientDo(t, client, http.MethodGet, srv.URL+"/login", "")
	if loginPage.StatusCode != http.StatusOK || !strings.Contains(loginPage.Body, "Operator sign-in") {
		t.Fatalf("login page %d %s", loginPage.StatusCode, loginPage.Body)
	}
	if loginPage.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}

	bad := clientDo(t, client, http.MethodPost, srv.URL+"/login", form(map[string]string{
		"csrf":     csrfValue(t, loginPage.Body),
		"username": "operator",
		"password": "wrong-password",
	}))
	if bad.StatusCode != http.StatusOK || !strings.Contains(bad.Body, "Invalid username or password.") {
		t.Fatalf("bad login %d %s", bad.StatusCode, bad.Body)
	}
	if strings.Contains(logs.String(), "wrong-password") || strings.Contains(logs.String(), "test-password") {
		t.Fatalf("password leaked into logs: %s", logs.String())
	}

	unauth := clientDo(t, client, http.MethodGet, srv.URL+"/groups", "")
	if unauth.StatusCode != http.StatusSeeOther || !strings.HasSuffix(unauth.Header.Get("Location"), "/login") {
		t.Fatalf("unauth groups %d %s", unauth.StatusCode, unauth.Header.Get("Location"))
	}

	goodLogin := clientDo(t, client, http.MethodPost, srv.URL+"/login", form(map[string]string{
		"csrf":     csrfValue(t, bad.Body),
		"username": "operator",
		"password": "test-password",
	}))
	if goodLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %d %s", goodLogin.StatusCode, goodLogin.Body)
	}

	groups := clientDo(t, client, http.MethodGet, srv.URL+"/groups", "")
	if groups.StatusCode != http.StatusOK || !strings.Contains(groups.Body, "No groups yet.") {
		t.Fatalf("groups %d %s", groups.StatusCode, groups.Body)
	}

	created := clientDo(t, client, http.MethodPost, srv.URL+"/groups", form(map[string]string{
		"csrf": csrfValue(t, groups.Body),
		"name": "Friends",
	}))
	if created.StatusCode != http.StatusSeeOther {
		t.Fatalf("create %d %s", created.StatusCode, created.Body)
	}
	groupURL := srv.URL + created.Header.Get("Location")

	detail := clientDo(t, client, http.MethodGet, groupURL, "")
	if !strings.Contains(detail.Body, "Friends") || strings.Contains(detail.Body, "start=join_") {
		t.Fatalf("detail before invite leaked or missing name: %s", detail.Body)
	}
	if !strings.Contains(detail.Body, "No members yet.") {
		t.Fatalf("members empty state missing: %s", detail.Body)
	}

	invited := clientDo(t, client, http.MethodPost, groupURL+"/invites", form(map[string]string{
		"csrf": csrfValue(t, detail.Body),
	}))
	if invited.StatusCode != http.StatusSeeOther {
		t.Fatalf("invite %d %s", invited.StatusCode, invited.Body)
	}

	withLink := clientDo(t, client, http.MethodGet, groupURL, "")
	link := deeplinkValue(t, withLink.Body)
	if !strings.Contains(link, "https://t.me/secret_santa_bot?start=join_") {
		t.Fatalf("link %s", link)
	}
	again := clientDo(t, client, http.MethodGet, groupURL, "")
	if !strings.Contains(again.Body, link) {
		t.Fatal("deeplink was hidden on reload")
	}

	revoked := clientDo(t, client, http.MethodPost, srv.URL+formAction(t, again.Body, "/invites/"), form(map[string]string{
		"csrf": csrfValue(t, again.Body),
	}))
	if revoked.StatusCode != http.StatusSeeOther {
		t.Fatalf("revoke %d %s", revoked.StatusCode, revoked.Body)
	}
	after := clientDo(t, client, http.MethodGet, groupURL, "")
	if !strings.Contains(after.Body, "Revoked") || !strings.Contains(after.Body, link) {
		t.Fatalf("revoked invite should keep its link: %s", after.Body)
	}

	archived := clientDo(t, client, http.MethodPost, groupURL+"/archive", form(map[string]string{
		"csrf": csrfValue(t, after.Body),
	}))
	if archived.StatusCode != http.StatusSeeOther {
		t.Fatalf("archive %d", archived.StatusCode)
	}
	finalPage := clientDo(t, client, http.MethodGet, groupURL, "")
	if strings.Contains(finalPage.Body, "New deeplink") || !strings.Contains(finalPage.Body, "Archived") {
		t.Fatalf("archived page: %s", finalPage.Body)
	}
}

func TestParseInviteOptions(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("max_uses=3&expires_on=2026-06-02"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	opts, err := parseInviteOptions(req, now)
	if err != nil || opts.MaxUses == nil || *opts.MaxUses != 3 || opts.ExpiresAt == nil {
		t.Fatalf("opts %+v err %v", opts, err)
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("expires_on=2026-05-01"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseInviteOptions(req, now); err != ErrInvalidInvite {
		t.Fatalf("got %v", err)
	}
}

func TestGamesPageShowsParticipantsWithoutPairs(t *testing.T) {
	deadline := time.Date(2026, 12, 2, 0, 0, 0, 0, games.Moscow)
	store := newMemStore()
	store.games = []GameDetail{{
		GameSummary: GameSummary{
			ID: 9, Name: "Office party", GroupID: 3, GroupName: "Friends",
			Status: "DRAWN", Confirmed: 1, ParticipantCount: 2,
			CreatedAt: time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC),
		},
		Description: "red paper",
		Deadline:    &deadline,
		Exchange:    "2026-12-25",
		InviteToken: "abcdefghijklmnopqr",
		People: []GameParticipant{
			{DisplayName: "Anna", Username: "anna", Status: "CONFIRMED", AssignmentState: "ASSIGNED"},
			{DisplayName: "Boris", Status: "PENDING", AssignmentState: "NOT_APPLICABLE"},
		},
	}}
	panel, err := New(store, Options{
		Username: "operator", Password: "test-password", BotUsername: "secret_santa_bot", SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(panel.Handler())
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	guest := clientDo(t, client, http.MethodGet, srv.URL+"/games", "")
	if guest.StatusCode != http.StatusSeeOther || guest.Header.Get("Location") != "/login" {
		t.Fatalf("guest %d %s", guest.StatusCode, guest.Header.Get("Location"))
	}
	loginPage := clientDo(t, client, http.MethodGet, srv.URL+"/login", "")
	signedIn := clientDo(t, client, http.MethodPost, srv.URL+"/login", form(map[string]string{
		"username": "operator",
		"password": "test-password",
		"csrf":     csrfValue(t, loginPage.Body),
	}))
	if signedIn.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %d", signedIn.StatusCode)
	}
	list := clientDo(t, client, http.MethodGet, srv.URL+"/games", "")
	for _, part := range []string{"Office party", "Friends", "Drawn", "1 / 2", "/games/9"} {
		if !strings.Contains(list.Body, part) {
			t.Fatalf("list missing %s", part)
		}
	}
	page := clientDo(t, client, http.MethodGet, srv.URL+"/games/9", "")
	if page.StatusCode != http.StatusOK {
		t.Fatalf("game %d", page.StatusCode)
	}
	for _, part := range []string{
		"Office party", "red paper", "01 Dec 2026", "25 Dec 2026",
		"https://t.me/secret_santa_bot?start=play_abcdefghijklmnopqr",
		"Anna", "@anna", "Confirmed", "Has a receiver",
		"Boris", "Pending", "Not in the draw",
	} {
		if !strings.Contains(page.Body, part) {
			t.Fatalf("page missing %s", part)
		}
	}
	if strings.Contains(page.Body, "Gives to") || strings.Contains(page.Body, "abcdefghijklmnopqr</") {
		t.Fatal("game page exposed a pair or the raw token")
	}
	missing := clientDo(t, client, http.MethodGet, srv.URL+"/games/4", "")
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing %d", missing.StatusCode)
	}
}

func TestUsersPageListsRegisteredPeople(t *testing.T) {
	started := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
	store := newMemStore()
	store.people = []RegisteredUser{{
		ID:          1,
		TelegramID:  4242,
		Username:    "anna",
		DisplayName: "Anna",
		DMStartedAt: &started,
		CreatedAt:   started,
		GroupCount:  1,
	}}
	panel, err := New(store, Options{
		Username:    "operator",
		Password:    "test-password",
		BotUsername: "secret_santa_bot",
		SessionTTL:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(panel.Handler())
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	guest := clientDo(t, client, http.MethodGet, srv.URL+"/users", "")
	if guest.StatusCode != http.StatusSeeOther || guest.Header.Get("Location") != "/login" {
		t.Fatalf("guest %d %s", guest.StatusCode, guest.Header.Get("Location"))
	}
	loginPage := clientDo(t, client, http.MethodGet, srv.URL+"/login", "")
	signedIn := clientDo(t, client, http.MethodPost, srv.URL+"/login", form(map[string]string{
		"username": "operator",
		"password": "test-password",
		"csrf":     csrfValue(t, loginPage.Body),
	}))
	if signedIn.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %d %s", signedIn.StatusCode, signedIn.Body)
	}
	page := clientDo(t, client, http.MethodGet, srv.URL+"/users", "")
	if page.StatusCode != http.StatusOK {
		t.Fatalf("users %d %s", page.StatusCode, page.Body)
	}
	for _, part := range []string{"Anna", "@anna", "4242", "20 Nov 2026 07:00 UTC", ">1<"} {
		if !strings.Contains(page.Body, part) {
			t.Fatalf("page missing %s: %s", part, page.Body)
		}
	}
}

type response struct {
	StatusCode int
	Header     http.Header
	Body       string
}

func clientDo(t *testing.T, client *http.Client, method, target, body string) response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{StatusCode: res.StatusCode, Header: res.Header, Body: string(buf)}
}

func TestLoginCookieSecureFollowsRequest(t *testing.T) {
	panel, err := New(newMemStore(), Options{
		Username:     "operator",
		Password:     "test-password",
		BotUsername:  "secret_santa_bot",
		SecureCookie: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := panel.Handler()

	plain := httptest.NewRecorder()
	handler.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/login", nil))
	if cookie := csrfCookieOf(t, plain); cookie.Secure {
		t.Fatal("secure cookie was set on plain HTTP")
	}

	httpsReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	httpsReq.Header.Set("X-Forwarded-Proto", "https")
	secure := httptest.NewRecorder()
	handler.ServeHTTP(secure, httpsReq)
	if cookie := csrfCookieOf(t, secure); !cookie.Secure {
		t.Fatal("cookie behind HTTPS was not marked Secure")
	}
}

func csrfCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == csrfCookie {
			return cookie
		}
	}
	t.Fatal("csrf cookie missing")
	return nil
}

func form(values map[string]string) string {
	data := url.Values{}
	for key, value := range values {
		data.Set(key, value)
	}
	return data.Encode()
}

func csrfValue(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="csrf" value="`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("csrf missing: %s", body)
	}
	rest := body[start+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("csrf unterminated")
	}
	return rest[:end]
}

func formAction(t *testing.T, body, prefix string) string {
	t.Helper()
	marker := `action="` + prefix
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("action %s missing", prefix)
	}
	rest := body[start+len(`action="`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("action unterminated")
	}
	return rest[:end]
}

func deeplinkValue(t *testing.T, body string) string {
	t.Helper()
	const marker = `value="https://t.me/`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("deeplink missing: %s", body)
	}
	rest := body[start+len(`value="`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("deeplink unterminated")
	}
	return rest[:end]
}

type memStore struct {
	mu       sync.Mutex
	sessions map[string]memSession
	groups   map[int64]*Group
	invites  map[int64]memInvite
	people   []RegisteredUser
	games    []GameDetail
	next     int64
}

type memSession struct {
	csrf    string
	expires time.Time
}

type memInvite struct {
	Invite
	groupID int64
}

func newMemStore() *memStore {
	return &memStore{
		sessions: map[string]memSession{},
		groups:   map[int64]*Group{},
		invites:  map[int64]memInvite{},
	}
}

func (m *memStore) CreateSession(context.Context, time.Duration) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	m.sessions[raw] = memSession{csrf: csrf, expires: time.Now().Add(time.Hour)}
	return raw, csrf, nil
}

func (m *memStore) DeleteSession(ctx context.Context, raw string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, raw)
	return nil
}

func (m *memStore) Session(ctx context.Context, raw string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[raw]
	if !ok || !sess.expires.After(time.Now()) {
		return Session{}, ErrNotFound
	}
	return Session{CSRF: sess.csrf, ExpiresAt: sess.expires}, nil
}

func (m *memStore) CreateGroup(ctx context.Context, name string) (Group, error) {
	name, err := NormalizeGroupName(name)
	if err != nil {
		return Group{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	g := Group{ID: m.next, Name: name, Status: "ACTIVE", CreatedAt: time.Now().UTC()}
	m.groups[g.ID] = &g
	return g, nil
}

func (m *memStore) ListGames(context.Context) ([]GameSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]GameSummary, 0, len(m.games))
	for _, game := range m.games {
		out = append(out, game.GameSummary)
	}
	return out, nil
}

func (m *memStore) Game(_ context.Context, id int64) (GameDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, game := range m.games {
		if game.ID == id {
			return game, nil
		}
	}
	return GameDetail{}, ErrNotFound
}

func (m *memStore) ListUsers(context.Context) ([]RegisteredUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]RegisteredUser(nil), m.people...), nil
}

func (m *memStore) ListGroups(context.Context) ([]GroupSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]GroupSummary, 0, len(m.groups))
	for _, g := range m.groups {
		item := GroupSummary{Group: *g}
		for _, inv := range m.invites {
			if inv.groupID == g.ID && inv.State == "Active" {
				item.ActiveInvites++
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *memStore) Group(ctx context.Context, id int64) (GroupDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return GroupDetail{}, ErrNotFound
	}
	detail := GroupDetail{Group: *g}
	for _, inv := range m.invites {
		if inv.groupID == id {
			detail.Invites = append(detail.Invites, inv.Invite)
		}
	}
	return detail, nil
}

func (m *memStore) ArchiveGroup(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return ErrNotFound
	}
	g.Status = "ARCHIVED"
	return nil
}

func (m *memStore) CreateInvite(ctx context.Context, groupID int64, opts InviteOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[groupID]
	if !ok {
		return ErrNotFound
	}
	if g.Status != "ACTIVE" {
		return ErrArchived
	}
	raw, _, err := newInviteToken()
	if err != nil {
		return err
	}
	m.next++
	inv := Invite{
		ID:        m.next,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: opts.ExpiresAt,
		MaxUses:   opts.MaxUses,
		Token:     raw,
	}
	inv.decorate(time.Now())
	m.invites[inv.ID] = memInvite{Invite: inv, groupID: groupID}
	return nil
}

func (m *memStore) RevokeInvite(ctx context.Context, inviteID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[inviteID]
	if !ok {
		return 0, ErrNotFound
	}
	now := time.Now().UTC()
	inv.RevokedAt = &now
	inv.decorate(now)
	m.invites[inviteID] = inv
	return inv.groupID, nil
}
