package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "admin_session"
	csrfCookie    = "admin_csrf"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the operator panel.
type Server struct {
	store        Store
	username     string
	password     string
	botUsername  string
	sessionTTL   time.Duration
	secureCookie bool
	log          *slog.Logger
	templates    *template.Template
}

// Options configures the panel.
type Options struct {
	Username     string
	Password     string
	BotUsername  string
	SessionTTL   time.Duration
	SecureCookie bool
	Log          *slog.Logger
}

type pageData struct {
	Title  string
	Authed bool
	CSRF   string
	Error  string
	Flash  string
}

type groupsPage struct {
	pageData
	Groups []GroupSummary
}

type groupPage struct {
	pageData
	Detail GroupDetail
}

// New builds the panel. It fails if the HTML templates cannot be parsed.
func New(store Store, opts Options) (*Server, error) {
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 30 * time.Minute
	}
	log := opts.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"formatTime":         formatTime,
		"formatOptionalTime": formatOptionalTime,
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{
		store:        store,
		username:     opts.Username,
		password:     opts.Password,
		botUsername:  opts.BotUsername,
		sessionTTL:   opts.SessionTTL,
		secureCookie: opts.SecureCookie,
		log:          log,
		templates:    tmpl,
	}, nil
}

// Handler is the admin HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /login", s.loginGET)
	mux.HandleFunc("POST /login", s.loginPOST)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /groups", s.groups)
	mux.HandleFunc("GET /groups/new", s.groupNew)
	mux.HandleFunc("POST /groups", s.groupCreate)
	mux.HandleFunc("GET /groups/{id}", s.groupShow)
	mux.HandleFunc("POST /groups/{id}/archive", s.groupArchive)
	mux.HandleFunc("POST /groups/{id}/invites", s.inviteCreate)
	mux.HandleFunc("POST /invites/{id}/revoke", s.inviteRevoke)
	return s.withHeaders(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) loginGET(w http.ResponseWriter, r *http.Request) {
	if _, _, err := s.currentSession(r); err == nil {
		http.Redirect(w, r, "/groups", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, "")
}

func (s *Server) loginPOST(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" || !sameSecret(r.FormValue("csrf"), cookie.Value) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	userOK := sameSecret(strings.TrimSpace(r.FormValue("username")), s.username)
	passOK := sameSecret(r.FormValue("password"), s.password)
	if !userOK || !passOK {
		s.log.Info("admin login failed")
		s.renderLogin(w, "Invalid username or password.")
		return
	}
	raw, _, err := s.store.CreateSession(r.Context(), s.sessionTTL)
	if err != nil {
		s.log.Error("create admin session", "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, csrfCookie, "", -time.Second)
	s.setCookie(w, sessionCookie, raw, s.sessionTTL)
	s.log.Info("admin login succeeded")
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	raw, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !sameSecret(r.FormValue("csrf"), sess.CSRF) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteSession(r.Context(), raw); err != nil {
		s.log.Error("delete admin session", "err", err)
	}
	s.setCookie(w, sessionCookie, "", -time.Second)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	groups, err := s.store.ListGroups(r.Context())
	if err != nil {
		s.log.Error("list groups", "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	s.render(w, "groups", groupsPage{
		pageData: pageData{Title: "Groups", Authed: true, CSRF: sess.CSRF},
		Groups:   groups,
	})
}

func (s *Server) groupNew(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	s.render(w, "group_form", pageData{Title: "New group", Authed: true, CSRF: sess.CSRF})
}

func (s *Server) groupCreate(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !sameSecret(r.FormValue("csrf"), sess.CSRF) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	group, err := s.store.CreateGroup(r.Context(), r.FormValue("name"))
	if err != nil {
		if errors.Is(err, ErrInvalidName) {
			s.render(w, "group_form", pageData{
				Title:  "New group",
				Authed: true,
				CSRF:   sess.CSRF,
				Error:  "Enter a group name, up to 80 characters.",
			})
			return
		}
		s.log.Error("create group", "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d", group.ID), http.StatusSeeOther)
}

func (s *Server) groupShow(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderGroup(w, r, id, "", true)
}

func (s *Server) groupArchive(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !sameSecret(r.FormValue("csrf"), sess.CSRF) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.store.ArchiveGroup(r.Context(), id); err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		s.log.Error("archive group", "err", err, "group_id", id)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d", id), http.StatusSeeOther)
}

func (s *Server) inviteCreate(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	raw, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !sameSecret(r.FormValue("csrf"), sess.CSRF) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	opts, err := parseInviteOptions(r, time.Now())
	if err != nil {
		s.renderGroup(w, r, id, "Check the use limit and expiry date.", false)
		return
	}
	if err := s.store.CreateInvite(r.Context(), raw, id, opts, s.botUsername); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, ErrArchived):
			s.renderGroup(w, r, id, "This group is archived.", false)
		default:
			s.log.Error("create invite", "err", err, "group_id", id)
			http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d", id), http.StatusSeeOther)
}

func (s *Server) inviteRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !sameSecret(r.FormValue("csrf"), sess.CSRF) {
		http.Error(w, "Form expired. Reload the page and try again.", http.StatusBadRequest)
		return
	}
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	groupID, err := s.store.RevokeInvite(r.Context(), id)
	if err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		s.log.Error("revoke invite", "err", err, "invite_id", id)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d", groupID), http.StatusSeeOther)
}

func (s *Server) renderGroup(w http.ResponseWriter, r *http.Request, id int64, formError string, takeFlash bool) {
	_, sess, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	detail, err := s.store.Group(r.Context(), id)
	if err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		s.log.Error("get group", "err", err, "group_id", id)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	var flash string
	if takeFlash {
		if cookie, cookieErr := r.Cookie(sessionCookie); cookieErr == nil {
			flash, err = s.store.TakeFlash(r.Context(), cookie.Value)
			if err != nil {
				s.log.Error("take flash", "err", err, "group_id", id)
				http.Error(w, "Something went wrong.", http.StatusInternalServerError)
				return
			}
		}
	}
	s.render(w, "group", groupPage{
		pageData: pageData{
			Title:  detail.Name,
			Authed: true,
			CSRF:   sess.CSRF,
			Error:  formError,
			Flash:  flash,
		},
		Detail: detail,
	})
}

func (s *Server) renderLogin(w http.ResponseWriter, message string) {
	csrf, err := randomToken(32)
	if err != nil {
		s.log.Error("login csrf", "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, csrfCookie, csrf, s.sessionTTL)
	s.render(w, "login", pageData{Title: "Sign in", CSRF: csrf, Error: message})
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (string, Session, bool) {
	raw, sess, err := s.currentSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return "", Session{}, false
	}
	return raw, sess, true
}

func (s *Server) currentSession(r *http.Request) (string, Session, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return "", Session{}, ErrNotFound
	}
	sess, err := s.store.Session(r.Context(), cookie.Value)
	if err != nil {
		return "", Session{}, err
	}
	return cookie.Value, sess, nil
}

func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form.", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("render template", "template", name, "err", err.Error())
	}
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie,
	}
	if ttl < 0 {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(0, 0)
	} else {
		cookie.MaxAge = int(ttl.Seconds())
		cookie.Expires = time.Now().Add(ttl)
	}
	http.SetCookie(w, cookie)
}

func (s *Server) withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src https://cdn.jsdelivr.net; img-src 'self'; script-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrNotFound
	}
	return id, nil
}

func parseInviteOptions(r *http.Request, now time.Time) (InviteOptions, error) {
	var opts InviteOptions
	if raw := strings.TrimSpace(r.FormValue("max_uses")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100000 {
			return InviteOptions{}, ErrInvalidInvite
		}
		opts.MaxUses = &n
	}
	if raw := strings.TrimSpace(r.FormValue("expires_on")); raw != "" {
		day, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return InviteOptions{}, ErrInvalidInvite
		}
		end := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
		if !end.After(now) {
			return InviteOptions{}, ErrInvalidInvite
		}
		opts.ExpiresAt = &end
	}
	return opts, nil
}

func sameSecret(got, want string) bool {
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("02 Jan 2006 15:04 UTC")
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "Never"
	}
	return formatTime(*t)
}
