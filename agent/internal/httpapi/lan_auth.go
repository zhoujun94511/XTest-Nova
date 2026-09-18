package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	lanSessionCookie  = "xtest_nova_lan_session"
	defaultSessionTTL = 12 * time.Hour
	defaultSessionCap = 128
)

// LANAuthConfig enables authentication on the primary HTTP handler.
type LANAuthConfig struct {
	Token       []byte
	SessionTTL  time.Duration
	MaxSessions int
	Now         func() time.Time
}

type lanSession struct {
	expires time.Time
	created time.Time
}

type lanAuth struct {
	tokenHash   [sha256.Size]byte
	sessionTTL  time.Duration
	maxSessions int
	now         func() time.Time
	mu          sync.Mutex
	sessions    map[string]lanSession
}

func newLANAuth(config LANAuthConfig) *lanAuth {
	if len(config.Token) == 0 {
		return nil
	}
	ttl := config.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	capacity := config.MaxSessions
	if capacity <= 0 {
		capacity = defaultSessionCap
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &lanAuth{
		tokenHash:   sha256.Sum256(config.Token),
		sessionTTL:  ttl,
		maxSessions: capacity,
		now:         now,
		sessions:    make(map[string]lanSession),
	}
}

func (a *lanAuth) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasQueryToken(r.URL.Query()) {
			a.reject(w)
			return
		}
		trustedLoopback := requestFromLoopback(r)
		if isPublicConsoleResource(r) {
			w.Header().Set("Referrer-Policy", "no-referrer")
		}
		switch {
		case r.URL.Path == "/v1/auth/lan/session":
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			switch r.Method {
			case http.MethodPost:
				if !sameOriginIfPresent(r) {
					a.reject(w)
					return
				}
				a.login(w, r)
			case http.MethodDelete:
				if !sameOriginIfPresent(r) {
					a.reject(w)
					return
				}
				a.logout(w, r)
			default:
				w.Header().Set("Allow", "POST, DELETE")
				fail(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			}
		case r.URL.Path == "/v1/auth/lan/status":
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				fail(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"enabled": true, "authenticated": trustedLoopback || a.authenticated(r)})
		case isPublicConsoleResource(r):
			next.ServeHTTP(w, r)
		default:
			if isWebSocketUpgrade(r) && !sameOriginIfPresent(r) {
				a.reject(w)
				return
			}
			if !trustedLoopback && !a.authenticated(r) {
				a.reject(w)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func (a *lanAuth) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var body struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil || !a.validToken([]byte(body.Token)) {
		a.reject(w)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		a.reject(w)
		return
	}
	sessionID, err := randomSessionID()
	if err != nil {
		fail(w, http.StatusInternalServerError, errors.New("could not create session"))
		return
	}
	now := a.now()
	a.mu.Lock()
	a.purgeExpiredLocked(now)
	for _, value := range sessionCookieValues(r) {
		delete(a.sessions, value)
	}
	for len(a.sessions) >= a.maxSessions {
		a.removeOldestLocked()
	}
	a.sessions[sessionID] = lanSession{created: now, expires: now.Add(a.sessionTTL)}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     lanSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  now.Add(a.sessionTTL),
	})
	writeJSON(w, http.StatusCreated, map[string]bool{"authenticated": true})
}

func (a *lanAuth) logout(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	for _, value := range sessionCookieValues(r) {
		delete(a.sessions, value)
	}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     lanSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (a *lanAuth) authenticated(r *http.Request) bool {
	authorization := r.Header.Values("Authorization")
	if len(authorization) > 1 {
		return false
	}
	if len(authorization) == 1 {
		token, ok := bearerToken(authorization[0])
		if !ok {
			return false
		}
		return a.validToken([]byte(token))
	}
	cookies := sessionCookieValues(r)
	if len(cookies) != 1 || cookies[0] == "" {
		return false
	}
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[cookies[0]]
	if !ok || !now.Before(session.expires) {
		delete(a.sessions, cookies[0])
		return false
	}
	return true
}

func sessionCookieValues(r *http.Request) []string {
	var values []string
	for _, cookie := range r.Cookies() {
		if cookie.Name == lanSessionCookie {
			values = append(values, cookie.Value)
		}
	}
	return values
}

func (a *lanAuth) validToken(token []byte) bool {
	hash := sha256.Sum256(token)
	return subtle.ConstantTimeCompare(hash[:], a.tokenHash[:]) == 1
}

func (a *lanAuth) reject(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	fail(w, http.StatusUnauthorized, errors.New("authentication required"))
}

func (a *lanAuth) purgeExpiredLocked(now time.Time) {
	for id, session := range a.sessions {
		if !now.Before(session.expires) {
			delete(a.sessions, id)
		}
	}
}

func (a *lanAuth) removeOldestLocked() {
	var oldestID string
	var oldest time.Time
	for id, session := range a.sessions {
		if oldestID == "" || session.created.Before(oldest) {
			oldestID, oldest = id, session.created
		}
	}
	delete(a.sessions, oldestID)
}

func randomSessionID() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func bearerToken(value string) (string, bool) {
	value = strings.Trim(value, " ")
	separator := strings.IndexByte(value, ' ')
	if separator <= 0 || !strings.EqualFold(value[:separator], "Bearer") {
		return "", false
	}
	token := strings.TrimLeft(value[separator+1:], " ")
	if token == "" || strings.IndexFunc(token, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}) >= 0 {
		return "", false
	}
	return token, true
}

func hasQueryToken(values url.Values) bool {
	for name := range values {
		normalized := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
		if normalized == "token" || normalized == "api_token" || normalized == "access_token" {
			return true
		}
	}
	return false
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func requestFromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func sameOriginIfPresent(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) > 1 {
		return false
	}
	origin := ""
	if len(origins) == 1 {
		origin = strings.TrimSpace(origins[0])
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	if !strings.EqualFold(parsed.Scheme, requestScheme) {
		return false
	}
	originHost, originPort, ok := normalizedOriginAuthority(parsed.Host, requestScheme)
	if !ok {
		return false
	}
	requestHost, requestPort, ok := normalizedOriginAuthority(r.Host, requestScheme)
	return ok && originHost == requestHost && originPort == requestPort
}

func normalizedOriginAuthority(authority, scheme string) (string, string, bool) {
	if strings.HasSuffix(authority, ":") {
		return "", "", false
	}
	parsed, err := url.Parse("//" + authority)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" {
		return "", "", false
	}
	host := parsed.Hostname()
	if host == "" {
		return "", "", false
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
	}
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", "", false
		}
	}
	return host, port, true
}

func isPublicConsoleResource(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	switch r.URL.Path {
	case "/",
		"/index.html",
		"/favicon.ico",
		"/app.css",
		"/app.js",
		"/placeholder.svg",
		"/assets/css/foundation.css",
		"/assets/css/shell.css",
		"/assets/css/components.css",
		"/assets/js/core.js",
		"/assets/js/renderers.js",
		"/assets/js/package-selector.js",
		"/assets/js/remote-control.js",
		"/assets/js/performance.js",
		"/assets/js/app.js",
		"/assets/media/favicon.svg",
		"/assets/media/placeholder.svg":
		return true
	}
	return false
}
