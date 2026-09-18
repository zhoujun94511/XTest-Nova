package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/configstore"
)

const testLANToken = "0123456789abcdef0123456789abcdef"

func TestLANAuthBearerAndPublicResources(t *testing.T) {
	api := newLANTestAPI(t, LANAuthConfig{Token: []byte(testLANToken)})
	handler := api.Primary()

	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if denied.Code != http.StatusUnauthorized || strings.Contains(denied.Body.String(), testLANToken) {
		t.Fatalf("unauthenticated response = %d %q", denied.Code, denied.Body.String())
	}

	allowedRequest := httptest.NewRequest(http.MethodGet, "/ping", nil)
	allowedRequest.Header.Set("Authorization", "Bearer "+testLANToken)
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusOK {
		t.Fatalf("Bearer request status = %d body=%s", allowed.Code, allowed.Body.String())
	}
	invalidRequest := httptest.NewRequest(http.MethodGet, "/ping", nil)
	invalidRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("x", len(testLANToken)))
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, invalidRequest)
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("invalid Bearer status = %d", invalid.Code)
	}

	for _, path := range []string{"/", "/assets/css/foundation.css", "/v1/auth/lan/status"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code == http.StatusUnauthorized {
			t.Fatalf("public login resource %s requires authentication", path)
		}
		if response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("public login resource %s Referrer-Policy=%q", path, response.Header().Get("Referrer-Policy"))
		}
	}

	queryRequest := httptest.NewRequest(http.MethodGet, "/ping?token="+testLANToken, nil)
	queryRequest.Header.Set("Authorization", "Bearer "+testLANToken)
	queryResponse := httptest.NewRecorder()
	handler.ServeHTTP(queryResponse, queryRequest)
	if queryResponse.Code != http.StatusUnauthorized || strings.Contains(queryResponse.Body.String(), testLANToken) {
		t.Fatalf("query token response = %d %q", queryResponse.Code, queryResponse.Body.String())
	}
}

func TestLANAuthTrustsOnlyTransportLoopback(t *testing.T) {
	handler := newLANTestAPI(t, LANAuthConfig{Token: []byte(testLANToken)}).Primary()
	for _, remoteAddr := range []string{"127.0.0.1:49152", "[::1]:49152"} {
		request := httptest.NewRequest(http.MethodGet, "/ping", nil)
		request.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("loopback %s status=%d body=%s", remoteAddr, response.Code, response.Body.String())
		}

		statusRequest := httptest.NewRequest(http.MethodGet, "/v1/auth/lan/status", nil)
		statusRequest.RemoteAddr = remoteAddr
		statusResponse := httptest.NewRecorder()
		handler.ServeHTTP(statusResponse, statusRequest)
		if statusResponse.Code != http.StatusOK ||
			!strings.Contains(statusResponse.Body.String(), `"authenticated":true`) {
			t.Fatalf("loopback status %s = %d %s", remoteAddr, statusResponse.Code, statusResponse.Body.String())
		}
	}

	spoofed := httptest.NewRequest(http.MethodGet, "/ping", nil)
	spoofed.RemoteAddr = "192.0.2.10:49152"
	spoofed.Header.Set("X-Forwarded-For", "127.0.0.1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, spoofed)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("forwarded loopback header bypassed LAN authentication: status=%d", response.Code)
	}
}

func TestLANAuthRejectsMalformedAuthorizationAndTokenQueries(t *testing.T) {
	handler := newLANTestAPI(t, LANAuthConfig{Token: []byte(testLANToken)}).Primary()
	for name, values := range map[string][]string{
		"missing":          nil,
		"wrong scheme":     {"Basic " + testLANToken},
		"missing token":    {"Bearer"},
		"extra field":      {"Bearer " + testLANToken + " extra"},
		"tab separator":    {"Bearer\t" + testLANToken},
		"combined schemes": {"Bearer " + testLANToken + ", Basic abc"},
		"duplicate fields": {"Bearer " + testLANToken, "Bearer " + testLANToken},
	} {
		request := httptest.NewRequest(http.MethodGet, "/ping", nil)
		for _, value := range values {
			request.Header.Add("Authorization", value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("%s authorization status=%d challenge=%q", name, response.Code, response.Header().Get("WWW-Authenticate"))
		}
	}
	for _, query := range []string{"TOKEN=x", "api-token=x", "Access_Token=x"} {
		request := httptest.NewRequest(http.MethodGet, "/ping?"+query, nil)
		request.Header.Set("Authorization", "Bearer "+testLANToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("query %q status=%d", query, response.Code)
		}
	}
}

func TestLANAuthProtectsPrimaryMediaAndLimitsPublicResources(t *testing.T) {
	handler := newLANTestAPI(t, LANAuthConfig{Token: []byte(testLANToken)}).Primary()
	for _, path := range []string{
		"/screenshot", "/takeScreenshot", "/packages/com.example/icon",
		"/raw/sdcard/image.png", "/archive/sdcard/results", "/scrcpy/screen/normal",
		"/assets/js/terminal.js", "/terminal.js", "/static/media/private.png",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("protected Primary resource %s status=%d", path, response.Code)
		}
	}
	for _, path := range []string{
		"/", "/index.html", "/favicon.ico", "/assets/css/foundation.css",
		"/assets/css/shell.css", "/assets/css/components.css", "/assets/js/core.js",
		"/assets/js/renderers.js", "/assets/js/package-selector.js",
		"/assets/js/remote-control.js", "/assets/js/performance.js",
		"/assets/js/app.js", "/assets/media/favicon.svg", "/assets/media/placeholder.svg",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("public login resource %s status=%d", path, response.Code)
		}
	}
}

func TestLANSessionCookieStatusLogoutAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	api := newLANTestAPI(t, LANAuthConfig{
		Token: []byte(testLANToken),
		Now:   func() time.Time { return now },
	})
	handler := api.Primary()

	loginRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`))
	loginRequest.Host = "agent.local:7912"
	loginRequest.Header.Set("Origin", "http://agent.local:7912")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusCreated {
		t.Fatalf("login status = %d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	if loginResponse.Header().Get("Cache-Control") != "no-store" ||
		loginResponse.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("login security headers = %v", loginResponse.Header())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v", cookies)
	}
	sessionCookie := cookies[0]
	if sessionCookie.Name != lanSessionCookie || !sessionCookie.HttpOnly ||
		sessionCookie.SameSite != http.SameSiteStrictMode || sessionCookie.Path != "/" ||
		sessionCookie.Secure || !sessionCookie.Expires.Equal(now.Add(12*time.Hour)) {
		t.Fatalf("unexpected session cookie: %#v", sessionCookie)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/v1/auth/lan/status", nil)
	statusRequest.AddCookie(sessionCookie)
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK ||
		statusResponse.Header().Get("Cache-Control") != "no-store" ||
		statusResponse.Header().Get("Referrer-Policy") != "no-referrer" ||
		!strings.Contains(statusResponse.Body.String(), `"enabled":true`) ||
		!strings.Contains(statusResponse.Body.String(), `"authenticated":true`) {
		t.Fatalf("authenticated status = %d %s", statusResponse.Code, statusResponse.Body.String())
	}

	protectedRequest := httptest.NewRequest(http.MethodGet, "/ping", nil)
	protectedRequest.AddCookie(sessionCookie)
	protectedResponse := httptest.NewRecorder()
	handler.ServeHTTP(protectedResponse, protectedRequest)
	if protectedResponse.Code != http.StatusOK {
		t.Fatalf("session request status = %d", protectedResponse.Code)
	}

	logoutRequest := httptest.NewRequest(http.MethodDelete, "/v1/auth/lan/session", nil)
	logoutRequest.AddCookie(sessionCookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusOK || len(logoutResponse.Result().Cookies()) != 1 ||
		logoutResponse.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout response = %d cookies=%#v", logoutResponse.Code, logoutResponse.Result().Cookies())
	}
	handler.ServeHTTP(httptest.NewRecorder(), logoutRequest)
	afterLogout := httptest.NewRecorder()
	handler.ServeHTTP(afterLogout, protectedRequest)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out session status = %d", afterLogout.Code)
	}

	secondLogin := httptest.NewRecorder()
	handler.ServeHTTP(secondLogin, httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`)))
	expiringCookie := secondLogin.Result().Cookies()[0]
	now = now.Add(12 * time.Hour)
	expiredRequest := httptest.NewRequest(http.MethodGet, "/ping", nil)
	expiredRequest.AddCookie(expiringCookie)
	expiredResponse := httptest.NewRecorder()
	handler.ServeHTTP(expiredResponse, expiredRequest)
	if expiredResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d", expiredResponse.Code)
	}
}

func TestLANLoginRejectsTrailingJSONAndRotatesSession(t *testing.T) {
	auth := newLANAuth(LANAuthConfig{Token: []byte(testLANToken)})
	handler := auth.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for name, body := range map[string]string{
		"second object": `{"token":"` + testLANToken + `"} {}`,
		"trailing text": `{"token":"` + testLANToken + `"} garbage`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(body)))
		if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 0 {
			t.Fatalf("%s status=%d cookies=%#v", name, response.Code, response.Result().Cookies())
		}
	}

	first := loginCookie(t, handler)
	rotationRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`))
	rotationRequest.AddCookie(first)
	rotationResponse := httptest.NewRecorder()
	handler.ServeHTTP(rotationResponse, rotationRequest)
	second := rotationResponse.Result().Cookies()[0]
	if first.Value == second.Value {
		t.Fatal("login reused the presented session ID")
	}
	for name, test := range map[string]struct {
		cookie *http.Cookie
		want   int
	}{
		"old session": {first, http.StatusUnauthorized},
		"new session": {second, http.StatusNoContent},
		"forged":      {&http.Cookie{Name: lanSessionCookie, Value: "chosen-by-client"}, http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.AddCookie(test.cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s status=%d, want %d", name, response.Code, test.want)
		}
	}
	duplicate := httptest.NewRequest(http.MethodGet, "/protected", nil)
	duplicate.AddCookie(second)
	duplicate.AddCookie(&http.Cookie{Name: lanSessionCookie, Value: "duplicate"})
	duplicateResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusUnauthorized {
		t.Fatalf("duplicate session cookies status=%d", duplicateResponse.Code)
	}
}

func TestLANSessionCapacityInvalidatesOldest(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	auth := newLANAuth(LANAuthConfig{
		Token:       []byte(testLANToken),
		MaxSessions: 1,
		Now:         func() time.Time { return now },
	})
	if auth == nil {
		t.Fatal("newLANAuth returned nil for a valid token")
	}
	handler := auth.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	first := loginCookie(t, handler)
	now = now.Add(time.Second)
	second := loginCookie(t, handler)
	for name, test := range map[string]struct {
		cookie *http.Cookie
		want   int
	}{
		"oldest": {first, http.StatusUnauthorized},
		"newest": {second, http.StatusNoContent},
	} {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.AddCookie(test.cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s session status = %d, want %d", name, response.Code, test.want)
		}
	}
}

func TestLANSessionConcurrentLoginAndLogout(t *testing.T) {
	auth := newLANAuth(LANAuthConfig{
		Token:       []byte(testLANToken),
		MaxSessions: 8,
	})
	if auth == nil {
		t.Fatal("newLANAuth returned nil for a valid token")
	}
	handler := auth.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	const workers = 32
	var wait sync.WaitGroup
	failures := make(chan string, workers)
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			login := httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`))
			loginResponse := httptest.NewRecorder()
			handler.ServeHTTP(loginResponse, login)
			cookies := loginResponse.Result().Cookies()
			if loginResponse.Code != http.StatusCreated || len(cookies) != 1 {
				failures <- "concurrent login failed"
				return
			}
			cookie := cookies[0]
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.AddCookie(cookie)
			handler.ServeHTTP(httptest.NewRecorder(), request)
			logout := httptest.NewRequest(http.MethodDelete, "/v1/auth/lan/session", nil)
			logout.AddCookie(cookie)
			handler.ServeHTTP(httptest.NewRecorder(), logout)
		}()
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if len(auth.sessions) > auth.maxSessions {
		t.Fatalf("concurrent sessions=%d, capacity=%d", len(auth.sessions), auth.maxSessions)
	}
}

func TestLANLoginAndWebSocketOrigin(t *testing.T) {
	auth := newLANAuth(LANAuthConfig{Token: []byte(testLANToken)})
	handler := auth.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	login := httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`))
	login.Host = "agent.local"
	login.Header.Set("Origin", "http://evil.local")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("cross-origin login status = %d", loginResponse.Code)
	}
	badLogin := httptest.NewRecorder()
	handler.ServeHTTP(badLogin, httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"wrong"}`)))
	if badLogin.Code != http.StatusUnauthorized || strings.Contains(badLogin.Body.String(), "wrong") {
		t.Fatalf("invalid login response = %d %q", badLogin.Code, badLogin.Body.String())
	}

	for name, test := range map[string]struct {
		origin string
		want   int
	}{
		"no origin API client": {"", http.StatusNoContent},
		"same origin":          {"http://agent.local", http.StatusNoContent},
		"cross origin":         {"http://evil.local", http.StatusUnauthorized},
		"wrong scheme":         {"https://agent.local", http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(http.MethodGet, "/socket", nil)
		request.Host = "agent.local"
		request.Header.Set("Upgrade", "websocket")
		request.Header.Set("Connection", "Upgrade")
		request.Header.Set("Authorization", "Bearer "+testLANToken)
		if test.origin != "" {
			request.Header.Set("Origin", test.origin)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s websocket status = %d, want %d", name, response.Code, test.want)
		}
	}
}

func TestLANWebSocketCookieAndNormalizedOrigin(t *testing.T) {
	handler := newLANAuth(LANAuthConfig{Token: []byte(testLANToken)}).handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	cookie := loginCookie(t, handler)
	for name, test := range map[string]struct {
		host   string
		origin string
		cookie *http.Cookie
		want   int
	}{
		"missing cookie":        {"agent.local", "http://agent.local", nil, http.StatusUnauthorized},
		"cookie same origin":    {"agent.local", "http://agent.local", cookie, http.StatusNoContent},
		"cookie cross origin":   {"agent.local", "http://evil.local", cookie, http.StatusUnauthorized},
		"default port":          {"agent.local:80", "http://agent.local", cookie, http.StatusNoContent},
		"equivalent IPv6":       {"[0:0:0:0:0:0:0:1]:7912", "http://[::1]:7912", cookie, http.StatusNoContent},
		"different IPv6 port":   {"[::1]:7912", "http://[::1]:7913", cookie, http.StatusUnauthorized},
		"origin with path":      {"agent.local", "http://agent.local/path", cookie, http.StatusUnauthorized},
		"origin with user info": {"agent.local", "http://user@agent.local", cookie, http.StatusUnauthorized},
		"origin trailing colon": {"agent.local", "http://agent.local:", cookie, http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(http.MethodGet, "/socket", nil)
		request.Host = test.host
		request.Header.Set("Upgrade", "websocket")
		request.Header.Set("Connection", "Upgrade")
		request.Header.Set("Origin", test.origin)
		if test.cookie != nil {
			request.AddCookie(test.cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s websocket status=%d, want %d", name, response.Code, test.want)
		}
	}
}

func TestLANAuthEndpointMethodsAndLogoutOrigin(t *testing.T) {
	handler := newLANAuth(LANAuthConfig{Token: []byte(testLANToken)}).handler(http.NotFoundHandler())
	for _, test := range []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodGet, "/v1/auth/lan/session", "POST, DELETE"},
		{http.MethodPut, "/v1/auth/lan/session", "POST, DELETE"},
		{http.MethodPost, "/v1/auth/lan/status", "GET"},
		{http.MethodDelete, "/v1/auth/lan/status", "GET"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != test.allow {
			t.Fatalf("%s %s status=%d Allow=%q", test.method, test.path, response.Code, response.Header().Get("Allow"))
		}
	}
	logout := httptest.NewRequest(http.MethodDelete, "/v1/auth/lan/session", nil)
	logout.Host = "agent.local"
	logout.Header.Set("Origin", "http://evil.local")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, logout)
	if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 0 {
		t.Fatalf("cross-origin logout status=%d cookies=%#v", response.Code, response.Result().Cookies())
	}
}

func TestLANAuthLeavesCompanionUnchangedAndProtectsLegacy(t *testing.T) {
	api := New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", true, LANAuthConfig{Token: []byte(testLANToken)})

	companion := httptest.NewRecorder()
	api.Companion().ServeHTTP(companion, httptest.NewRequest(http.MethodGet, "/health", nil))
	if companion.Code != http.StatusOK {
		t.Fatalf("Companion health status = %d", companion.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/term", nil)
	response := httptest.NewRecorder()
	api.Primary().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated LAN terminal status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/term", nil)
	request.Header.Set("Authorization", "Bearer "+testLANToken)
	response = httptest.NewRecorder()
	api.Primary().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated LAN terminal status = %d", response.Code)
	}
}

func newLANTestAPI(t *testing.T, config LANAuthConfig) *API {
	t.Helper()
	return New(fakeDevice{}, &fakeRunner{}, configstore.NewEmpty(t.TempDir()+"/config.json"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", false, config)
}

func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/lan/session", strings.NewReader(`{"token":"`+testLANToken+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login failed: status=%d body=%s", response.Code, response.Body.String())
	}
	return response.Result().Cookies()[0]
}
