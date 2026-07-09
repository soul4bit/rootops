package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/soul4bit/rootops/internal/auth"
	"github.com/soul4bit/rootops/internal/config"
	"github.com/soul4bit/rootops/internal/email"
	"github.com/soul4bit/rootops/internal/storage"
)

const (
	maxBodyBytes  = 16 * 1024
	sessionCookie = "rootops_session"
)

type Server struct {
	cfg       config.Config
	store     *storage.Store
	templates *template.Template
	email     *email.Sender
	limiter   *auth.RateLimiter
	mux       *http.ServeMux
	assets    http.Handler
}

type sessionState struct {
	token   string
	session *storage.Session
}

func NewServer(cfg config.Config, store *storage.Store) (*Server, error) {
	dashboardPath := filepath.Join(cfg.ProjectRoot, "web", "templates", "dashboard.html")
	templates, err := template.ParseFiles(dashboardPath)
	if err != nil {
		return nil, err
	}

	server := &Server{
		cfg:       cfg,
		store:     store,
		templates: templates,
		email: email.NewSender(email.Config{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}),
		limiter: auth.NewRateLimiter(5 * time.Minute),
		mux:     http.NewServeMux(),
		assets:  http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(cfg.ProjectRoot, "assets")))),
	}
	server.routes()

	return server, nil
}

func (server *Server) Handler() http.Handler {
	return server.securityHeaders(server.logRequests(server.mux))
}

func (server *Server) routes() {
	server.mux.HandleFunc("/api/auth/csrf", server.handleCSRF)
	server.mux.HandleFunc("/api/auth/me", server.handleMe)
	server.mux.HandleFunc("/api/auth/register", server.handleRegister)
	server.mux.HandleFunc("/api/auth/login", server.handleLogin)
	server.mux.HandleFunc("/api/auth/logout", server.handleLogoutJSON)
	server.mux.HandleFunc("/verify-email", server.handleVerifyEmail)
	server.mux.HandleFunc("/dashboard", server.handleDashboard)
	server.mux.HandleFunc("/login", server.redirectToAuth("login"))
	server.mux.HandleFunc("/register", server.redirectToAuth("register"))
	server.mux.HandleFunc("/logout", server.handleLogoutForm)
	server.mux.HandleFunc("/", server.handleStatic)
}

func (server *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}

	state, err := server.currentSession(r, true)
	if err != nil {
		server.errorJSON(w, http.StatusInternalServerError, "Не удалось создать защищённую сессию.")
		return
	}

	server.setSessionCookie(w, state.token, int(server.cfg.SessionTTL.Seconds()))
	server.writeJSON(w, http.StatusOK, map[string]string{"csrfToken": state.session.CSRFToken})
}

func (server *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}

	state, err := server.currentSession(r, false)
	if err != nil || state == nil || !state.session.UserID.Valid {
		server.writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	user, err := server.store.UserByID(r.Context(), state.session.UserID.Int64)
	if err != nil || user.EmailVerifiedAt == 0 {
		server.writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

func (server *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}

	payload, ok := server.readAuthPayload(w, r)
	if !ok {
		return
	}

	state, ok := server.requireCSRF(w, r, payload.CSRFToken)
	if !ok {
		return
	}

	if !server.limiter.Allow(server.clientKey(r, "register"), 5) {
		server.errorJSON(w, http.StatusTooManyRequests, "Слишком много попыток. Попробуйте позже.")
		return
	}

	name := strings.TrimSpace(payload.Name)
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	password := payload.Password

	if validationError := auth.ValidateRegistration(name, email, password); validationError != "" {
		server.errorJSON(w, http.StatusBadRequest, validationError)
		return
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		server.errorJSON(w, http.StatusInternalServerError, "Не удалось подготовить пароль.")
		return
	}

	now := time.Now().Unix()
	userID, err := server.store.CreateUser(r.Context(), name, email, passwordHash, now)
	if err != nil {
		if storage.IsDuplicate(err) {
			server.errorJSON(w, http.StatusConflict, "Аккаунт с таким email уже существует.")
			return
		}
		server.errorJSON(w, http.StatusInternalServerError, "Не удалось создать аккаунт.")
		return
	}

	user := &storage.User{
		ID:           userID,
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
		CreatedAt:    now,
	}

	message := "Аккаунт создан. Мы отправили письмо со ссылкой для подтверждения почты."
	if err := server.issueEmailVerification(r, user); err != nil {
		log.Printf("send verification email: %v", err)
		message = "Аккаунт создан, но письмо сейчас не отправилось. Попробуй войти позже - мы отправим ссылку ещё раз."
	}

	server.writeJSON(w, http.StatusCreated, map[string]any{
		"ok":                   true,
		"requiresVerification": true,
		"message":              message,
		"csrfToken":            state.session.CSRFToken,
	})
}

func (server *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}

	payload, ok := server.readAuthPayload(w, r)
	if !ok {
		return
	}

	state, ok := server.requireCSRF(w, r, payload.CSRFToken)
	if !ok {
		return
	}

	if !server.limiter.Allow(server.clientKey(r, "login"), 8) {
		server.errorJSON(w, http.StatusTooManyRequests, "Слишком много попыток. Попробуйте позже.")
		return
	}

	email := strings.ToLower(strings.TrimSpace(payload.Email))
	user, err := server.store.UserByEmail(r.Context(), email)
	if err != nil || !auth.VerifyPassword(payload.Password, user.PasswordHash) {
		server.errorJSON(w, http.StatusUnauthorized, "Неверный email или пароль.")
		return
	}
	if user.EmailVerifiedAt == 0 {
		message := "Почта не подтверждена. Мы отправили новую ссылку для подтверждения."
		if err := server.issueEmailVerification(r, user); err != nil {
			log.Printf("resend verification email: %v", err)
			message = "Почта не подтверждена. Сейчас не удалось отправить письмо, попробуй позже."
		}
		server.errorJSON(w, http.StatusForbidden, message)
		return
	}

	userID := user.ID
	token, csrfToken, err := server.createSession(r, &userID, state.token)
	if err != nil {
		server.errorJSON(w, http.StatusInternalServerError, "Не удалось открыть сессию.")
		return
	}

	server.setSessionCookie(w, token, int(server.cfg.SessionTTL.Seconds()))
	server.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrfToken": csrfToken})
}

func (server *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}

	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		http.Redirect(w, r, "/?auth=login&verify=missing", http.StatusFound)
		return
	}

	user, err := server.store.VerifyEmailByToken(r.Context(), auth.TokenHash(rawToken), time.Now().Unix())
	if err != nil {
		if !storage.IsNotFound(err) {
			log.Printf("verify email: %v", err)
		}
		http.Redirect(w, r, "/?auth=login&verify=expired", http.StatusFound)
		return
	}

	userID := user.ID
	oldToken := readSessionCookie(r)
	token, _, err := server.createSession(r, &userID, oldToken)
	if err != nil {
		log.Printf("create verified session: %v", err)
		http.Redirect(w, r, "/?auth=login&verify=success", http.StatusFound)
		return
	}

	server.setSessionCookie(w, token, int(server.cfg.SessionTTL.Seconds()))
	http.Redirect(w, r, "/dashboard?verified=1", http.StatusFound)
}

func (server *Server) handleLogoutJSON(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}

	payload, ok := server.readCSRFPayload(w, r)
	if !ok {
		return
	}

	state, ok := server.requireCSRF(w, r, payload.CSRFToken)
	if !ok {
		return
	}

	_ = server.store.DeleteSession(r.Context(), auth.TokenHash(state.token))
	server.clearSessionCookie(w)
	server.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (server *Server) handleLogoutForm(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		server.clearSessionCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !allowMethod(w, r, http.MethodPost) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/?auth=login", http.StatusSeeOther)
		return
	}

	state, ok := server.requireCSRF(w, r, r.FormValue("csrfToken"))
	if !ok {
		return
	}

	_ = server.store.DeleteSession(r.Context(), auth.TokenHash(state.token))
	server.clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (server *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}

	state, err := server.currentSession(r, false)
	if err != nil || state == nil || !state.session.UserID.Valid {
		http.Redirect(w, r, "/?auth=login", http.StatusFound)
		return
	}

	user, err := server.store.UserByID(r.Context(), state.session.UserID.Int64)
	if err != nil {
		http.Redirect(w, r, "/?auth=login", http.StatusFound)
		return
	}
	if user.EmailVerifiedAt == 0 {
		_ = server.store.DeleteSession(r.Context(), auth.TokenHash(state.token))
		server.clearSessionCookie(w)
		http.Redirect(w, r, "/?auth=login&verify=required", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	data := dashboardData{
		UserName:     user.Name,
		UserEmail:    user.Email,
		UserInitials: initials(user.Name),
		CSRFToken:    state.session.CSRFToken,
	}
	if err := server.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("render dashboard: %v", err)
	}
}

func (server *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	if r.URL.Path == "/" {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(server.cfg.ProjectRoot, "index.html"))
		return
	}

	if strings.HasPrefix(r.URL.Path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		server.assets.ServeHTTP(w, r)
		return
	}

	http.NotFound(w, r)
}

func (server *Server) redirectToAuth(mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/?auth=%s", mode), http.StatusFound)
	}
}

func (server *Server) issueEmailVerification(r *http.Request, user *storage.User) error {
	token, err := auth.NewToken(40)
	if err != nil {
		return err
	}

	now := time.Now()
	if err := server.store.CreateEmailVerificationToken(
		r.Context(),
		user.ID,
		auth.TokenHash(token),
		now.Unix(),
		now.Add(server.cfg.VerifyTTL).Unix(),
	); err != nil {
		return err
	}

	link := server.emailVerificationURL(r, token)
	if !server.email.Configured() {
		log.Printf("email sender is not configured; verification link for %s: %s", user.Email, link)
		return nil
	}

	return server.email.Send(user.Email, "Подтверждение почты RootOPS", verificationEmailBody(user.Name, link, server.cfg.VerifyTTL))
}

func (server *Server) emailVerificationURL(r *http.Request, token string) string {
	return server.publicBaseURL(r) + "/verify-email?token=" + url.QueryEscape(token)
}

func (server *Server) publicBaseURL(r *http.Request) string {
	if server.cfg.PublicURL != "" {
		return server.cfg.PublicURL
	}

	scheme := "http"
	if forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwardedProto == "http" || forwardedProto == "https" {
		scheme = forwardedProto
	} else if r.TLS != nil {
		scheme = "https"
	}

	host := r.Host
	if host == "" {
		host = server.cfg.Addr
	}
	return scheme + "://" + host
}

func verificationEmailBody(name string, link string, ttl time.Duration) string {
	greetingName := strings.TrimSpace(name)
	if greetingName == "" {
		greetingName = "друг"
	}

	hours := int(ttl.Hours())
	if hours <= 0 {
		hours = 24
	}

	return fmt.Sprintf(`Привет, %s!

Подтверди почту, чтобы открыть личный DevOps-сервер в RootOPS:
%s

Ссылка действует %d часа. Если ты не регистрировался в RootOPS, просто удали это письмо.

RootOPS`, greetingName, link, hours)
}

func (server *Server) currentSession(r *http.Request, create bool) (*sessionState, error) {
	token := readSessionCookie(r)
	if token != "" {
		session, err := server.store.SessionByHash(r.Context(), auth.TokenHash(token), time.Now().Unix())
		if err == nil {
			return &sessionState{token: token, session: session}, nil
		}
		if err != nil && !storage.IsNotFound(err) {
			return nil, err
		}
	}

	if !create {
		return nil, nil
	}

	token, csrfToken, err := server.createSession(r, nil, "")
	if err != nil {
		return nil, err
	}
	session, err := server.store.SessionByHash(r.Context(), auth.TokenHash(token), time.Now().Unix())
	if err != nil {
		return nil, err
	}

	session.CSRFToken = csrfToken
	return &sessionState{token: token, session: session}, nil
}

func (server *Server) createSession(r *http.Request, userID *int64, oldToken string) (string, string, error) {
	token, err := auth.NewToken(48)
	if err != nil {
		return "", "", err
	}
	csrfToken, err := auth.NewToken(32)
	if err != nil {
		return "", "", err
	}

	oldHash := ""
	if oldToken != "" {
		oldHash = auth.TokenHash(oldToken)
	}

	now := time.Now().Unix()
	expiresAt := time.Now().Add(server.cfg.SessionTTL).Unix()
	if err := server.store.ReplaceSession(r.Context(), oldHash, auth.TokenHash(token), userID, csrfToken, now, expiresAt); err != nil {
		return "", "", err
	}

	return token, csrfToken, nil
}

func (server *Server) requireCSRF(w http.ResponseWriter, r *http.Request, payloadToken string) (*sessionState, bool) {
	state, err := server.currentSession(r, false)
	if err != nil {
		server.errorJSON(w, http.StatusInternalServerError, "Не удалось проверить сессию.")
		return nil, false
	}

	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = payloadToken
	}

	if state == nil || !auth.SecureEqual(token, state.session.CSRFToken) {
		server.errorJSON(w, http.StatusForbidden, "CSRF-токен недействителен.")
		return nil, false
	}

	return state, true
}

func (server *Server) readAuthPayload(w http.ResponseWriter, r *http.Request) (authPayload, bool) {
	var payload authPayload
	if !server.readJSON(w, r, &payload) {
		return payload, false
	}
	return payload, true
}

func (server *Server) readCSRFPayload(w http.ResponseWriter, r *http.Request) (csrfPayload, bool) {
	var payload csrfPayload
	if !server.readJSON(w, r, &payload) {
		return payload, false
	}
	return payload, true
}

func (server *Server) readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		server.errorJSON(w, http.StatusBadRequest, "Некорректный JSON.")
		return false
	}
	return true
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (server *Server) errorJSON(w http.ResponseWriter, status int, message string) {
	server.writeJSON(w, status, map[string]string{"error": message})
}

func (server *Server) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   server.cfg.CookieSecure,
	})
}

func (server *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   server.cfg.CookieSecure,
	})
}

func (server *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (server *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func readSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (server *Server) clientKey(r *http.Request, action string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host + ":" + action
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "RO"
	}

	letters := make([]rune, 0, 2)
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) > 0 {
			letters = append(letters, []rune(strings.ToUpper(string(runes[0])))[0])
		}
		if len(letters) == 2 {
			break
		}
	}

	if len(letters) == 0 {
		return "RO"
	}
	return string(letters)
}

type authPayload struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	CSRFToken string `json:"csrfToken"`
}

type csrfPayload struct {
	CSRFToken string `json:"csrfToken"`
}

type dashboardData struct {
	UserName     string
	UserEmail    string
	UserInitials string
	CSRFToken    string
}
