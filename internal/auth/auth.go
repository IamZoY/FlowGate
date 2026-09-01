package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	cookieName = "flowgate_session"
	sessionTTL = 24 * time.Hour
	// confirmHeader carries the re-entered admin password required on
	// every mutating API request (create/update/delete).
	confirmHeader = "X-Confirm-Password"
)

// Service implements session-cookie auth for the dashboard plus password
// re-confirmation for mutating API requests. Sessions are held in memory,
// so a server restart logs everyone out.
type Service struct {
	enabled  bool
	username string
	password string
	secure   bool // set Secure on cookies (TLS enabled)

	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
}

// NewService creates the auth service. When enabled is false every check
// passes through, preserving the zero-config behaviour.
func NewService(enabled bool, username, password string, secureCookies bool) *Service {
	return &Service{
		enabled:  enabled,
		username: username,
		password: password,
		secure:   secureCookies,
		sessions: make(map[string]time.Time),
	}
}

// Enabled reports whether dashboard auth is turned on.
func (s *Service) Enabled() bool { return s.enabled }

// Router returns the /api/auth sub-router. These routes must be reachable
// without a session so the login page can work.
func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Post("/login", s.handleLogin)
	r.Post("/logout", s.handleLogout)
	r.Get("/session", s.handleSession)
	return r
}

// ---------- handlers ---------------------------------------------------------

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.enabled {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) == 1
	if !userOK || !passOK {
		time.Sleep(time.Second) // slow down brute-force attempts
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	token := newToken()
	now := time.Now()
	s.mu.Lock()
	s.gcLocked(now)
	s.sessions[token] = now.Add(sessionTTL)
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleSession lets the frontend discover whether auth is on and whether
// the current browser session is valid.
func (s *Service) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_enabled":  s.enabled,
		"authenticated": s.Authed(r),
	})
}

// ---------- checks & middleware ----------------------------------------------

// Authed reports whether the request carries a valid session cookie.
// Always true when auth is disabled.
func (s *Service) Authed(r *http.Request) bool {
	if !s.enabled {
		return true
	}
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[c.Value]
	if !ok {
		return false
	}
	if now.After(exp) {
		delete(s.sessions, c.Value)
		return false
	}
	// Sliding expiry: keep active sessions alive.
	s.sessions[c.Value] = now.Add(sessionTTL)
	return true
}

// RequireSession rejects unauthenticated API requests with 401.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Authed(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireConfirm gates every mutating request (POST/PUT/PATCH/DELETE) behind
// a re-entered admin password sent in the X-Confirm-Password header.
func (s *Service) RequireConfirm(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.enabled {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			pw := r.Header.Get(confirmHeader)
			if pw == "" {
				http.Error(w, "password confirmation required", http.StatusPreconditionRequired)
				return
			}
			if subtle.ConstantTimeCompare([]byte(pw), []byte(s.password)) != 1 {
				time.Sleep(time.Second)
				http.Error(w, "incorrect password", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ProtectPages wraps the SPA handler: unauthenticated browsers are redirected
// to /login, which serves the embedded login page.
func (s *Service) ProtectPages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.enabled {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/login" || r.URL.Path == "/login.html" {
			if s.Authed(r) {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/login.html"
			next.ServeHTTP(w, r2)
			return
		}
		if !s.Authed(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------------------------------------------------------

// gcLocked drops expired sessions. Caller must hold s.mu.
func (s *Service) gcLocked(now time.Time) {
	for tok, exp := range s.sessions {
		if now.After(exp) {
			delete(s.sessions, tok)
		}
	}
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
