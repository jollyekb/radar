package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// newTestOIDCHandler creates a minimal OIDCHandler for testing validation paths.
// The provider/oauth/verifier fields are nil — only use for tests that return
// before token exchange.
func newTestOIDCHandler() *OIDCHandler {
	return &OIDCHandler{
		cfg: Config{
			Mode:   "oidc",
			Secret: "test-secret",
		},
	}
}

func TestOIDCCallback_MissingStateCookie(t *testing.T) {
	h := newTestOIDCHandler()
	r := httptest.NewRequest("GET", "/auth/callback?state=abc&code=xyz", nil)
	// No state cookie set
	w := httptest.NewRecorder()

	h.HandleCallback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if body := w.Body.String(); body == "" {
		t.Error("expected error message in body")
	}
}

func TestOIDCCallback_MismatchedState(t *testing.T) {
	h := newTestOIDCHandler()
	r := httptest.NewRequest("GET", "/auth/callback?state=wrong&code=xyz", nil)
	r.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "expected"})
	w := httptest.NewRecorder()

	h.HandleCallback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestOIDCCallback_MissingCode(t *testing.T) {
	h := newTestOIDCHandler()
	r := httptest.NewRequest("GET", "/auth/callback?state=abc", nil)
	r.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "abc"})
	w := httptest.NewRecorder()

	h.HandleCallback(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleLogout_NoEndSessionEndpoint(t *testing.T) {
	h := newTestOIDCHandler()
	// endSessionEndpoint is empty by default
	r := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "logged out" {
		t.Errorf("status = %q, want %q", resp["status"], "logged out")
	}
	if _, ok := resp["redirectTo"]; ok {
		t.Error("redirectTo should not be present when end_session_endpoint is empty")
	}
}

func TestHandleLogout_WithEndSessionEndpoint(t *testing.T) {
	h := newTestOIDCHandler()
	h.endSessionEndpoint = "https://idp.example.com/logout"
	h.cfg.OIDCClientID = "radar-client"

	// Create a session cookie with an ID token
	user := &User{Username: "alice"}
	cookie := CreateSessionCookieWithIDToken(user, "my-id-token", h.cfg.Secret, 1*time.Hour, false)

	r := httptest.NewRequest("GET", "/auth/logout", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	h.HandleLogout(w, r)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	redirectTo := resp["redirectTo"]
	if redirectTo == "" {
		t.Fatal("redirectTo should be present")
	}
	if !strings.HasPrefix(redirectTo, "https://idp.example.com/logout") {
		t.Errorf("redirectTo = %q, want prefix https://idp.example.com/logout", redirectTo)
	}
	if !strings.Contains(redirectTo, "id_token_hint=my-id-token") {
		t.Errorf("redirectTo should contain id_token_hint, got %q", redirectTo)
	}
	// Should not contain client_id when id_token_hint is present
	if strings.Contains(redirectTo, "client_id=") {
		t.Errorf("redirectTo should not contain client_id when id_token_hint is present")
	}

	// Session cookie should be cleared
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == DefaultCookieName && c.MaxAge == -1 {
			found = true
		}
	}
	if !found {
		t.Error("session cookie should be cleared")
	}
}

func TestHandleLogout_WithPostLogoutRedirectURL(t *testing.T) {
	h := newTestOIDCHandler()
	h.endSessionEndpoint = "https://idp.example.com/logout"
	h.cfg.OIDCPostLogoutRedirectURL = "https://radar.example.com/"

	r := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, r)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	redirectTo := resp["redirectTo"]
	if !strings.Contains(redirectTo, "post_logout_redirect_uri=") {
		t.Errorf("redirectTo should contain post_logout_redirect_uri, got %q", redirectTo)
	}
}

func TestHandleLogout_NoIDTokenInCookie(t *testing.T) {
	h := newTestOIDCHandler()
	h.endSessionEndpoint = "https://idp.example.com/logout"
	h.cfg.OIDCClientID = "radar-client"

	// Session cookie without ID token (old session from before upgrade)
	user := &User{Username: "alice"}
	cookie := CreateSessionCookie(user, h.cfg.Secret, 1*time.Hour, false)

	r := httptest.NewRequest("GET", "/auth/logout", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()

	h.HandleLogout(w, r)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	redirectTo := resp["redirectTo"]
	if redirectTo == "" {
		t.Fatal("redirectTo should be present even without id_token")
	}
	// Should fall back to client_id
	if !strings.Contains(redirectTo, "client_id=radar-client") {
		t.Errorf("redirectTo should contain client_id fallback, got %q", redirectTo)
	}
	if strings.Contains(redirectTo, "id_token_hint=") {
		t.Errorf("redirectTo should not contain id_token_hint when cookie has no token")
	}
}

func TestHandleLogout_SetsForceLoginCookie(t *testing.T) {
	h := newTestOIDCHandler()
	// No end_session_endpoint — simulates Google
	r := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, r)

	// Should set the force-login cookie
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcForceLoginCookieName && c.Value == "1" {
			found = true
			if c.MaxAge != 300 {
				t.Errorf("force-login cookie MaxAge = %d, want 300", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("logout should set force-login cookie")
	}
}

func TestHandleLogin_ForceLoginPrompt(t *testing.T) {
	h := newTestOIDCHandler()
	// Set up minimal oauth config so AuthCodeURL works
	h.oauth = oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
		},
		RedirectURL: "http://localhost:9280/auth/callback",
		Scopes:      []string{"openid"},
	}

	// Request with force-login cookie set
	r := httptest.NewRequest("GET", "/auth/login", nil)
	r.AddCookie(&http.Cookie{Name: oidcForceLoginCookieName, Value: "1"})
	w := httptest.NewRecorder()

	h.HandleLogin(w, r)

	// Should redirect to IdP with prompt=login
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	location := w.Header().Get("Location")
	if !strings.Contains(location, "prompt=login") {
		t.Errorf("redirect URL should contain prompt=login, got %q", location)
	}

	// Should clear the force-login cookie
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcForceLoginCookieName && c.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("force-login cookie should be cleared after use")
	}
}

func TestHandleLogin_NoForceLoginWithoutCookie(t *testing.T) {
	h := newTestOIDCHandler()
	h.oauth = oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
		},
		RedirectURL: "http://localhost:9280/auth/callback",
		Scopes:      []string{"openid"},
	}

	// Request WITHOUT force-login cookie
	r := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()

	h.HandleLogin(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	location := w.Header().Get("Location")
	if strings.Contains(location, "prompt=login") {
		t.Errorf("redirect URL should NOT contain prompt=login on normal login, got %q", location)
	}
}
