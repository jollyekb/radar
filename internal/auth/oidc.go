package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const oidcStateCookieName = "radar_oidc_state"

// OIDCHandler handles the OIDC login flow
type OIDCHandler struct {
	cfg                Config
	provider           *oidc.Provider
	oauth              oauth2.Config
	verifier           *oidc.IDTokenVerifier
	endSessionEndpoint string // from OIDC discovery; empty if IdP doesn't support RP-Initiated Logout
}

// NewOIDCHandler creates a new OIDC handler. Returns an error if the provider
// cannot be discovered (network error, invalid issuer URL, etc.).
func NewOIDCHandler(ctx context.Context, cfg Config) (*OIDCHandler, error) {
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, err
	}

	oauthCfg := oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})

	// Extract end_session_endpoint from OIDC discovery document for RP-Initiated Logout
	var providerClaims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&providerClaims); err != nil {
		log.Printf("[oidc] Warning: failed to parse end_session_endpoint from discovery document: %v", err)
	} else if providerClaims.EndSessionEndpoint != "" {
		log.Printf("[oidc] RP-Initiated Logout enabled (end_session_endpoint discovered)")
	} else {
		log.Printf("[oidc] RP-Initiated Logout not available (IdP does not advertise end_session_endpoint)")
	}

	return &OIDCHandler{
		cfg:                cfg,
		provider:           provider,
		oauth:              oauthCfg,
		verifier:           verifier,
		endSessionEndpoint: providerClaims.EndSessionEndpoint,
	}, nil
}

// HandleLogin redirects to the OIDC provider for authentication
func (h *OIDCHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate random state nonce and store in a short-lived cookie for CSRF protection
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Printf("[oidc] Failed to generate state nonce: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

// HandleCallback processes the OIDC callback after authentication
func (h *OIDCHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Verify state against cookie to prevent CSRF
	stateCookie, err := r.Cookie(oidcStateCookieName)
	if err != nil || stateCookie.Value == "" {
		http.Error(w, "missing state cookie — please retry login", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}
	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   oidcStateCookieName,
		Path:   "/",
		MaxAge: -1,
	})

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	token, err := h.oauth.Exchange(ctx, code)
	if err != nil {
		log.Printf("[oidc] Token exchange failed: %v", err)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Extract and verify ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		log.Printf("[oidc] No id_token in token response")
		http.Error(w, "authentication failed: no id_token", http.StatusInternalServerError)
		return
	}

	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		log.Printf("[oidc] Token verification failed: %v", err)
		http.Error(w, "authentication failed: invalid token", http.StatusUnauthorized)
		return
	}

	// Extract claims
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		log.Printf("[oidc] Failed to parse claims: %v", err)
		http.Error(w, "authentication failed: invalid claims", http.StatusInternalServerError)
		return
	}

	// Extract username (prefer email, fall back to sub)
	username := ""
	if email, ok := claims["email"].(string); ok && email != "" {
		username = email
	} else if sub, ok := claims["sub"].(string); ok {
		username = sub
	}

	if username == "" {
		log.Printf("[oidc] No username claim (email or sub) in ID token")
		http.Error(w, "authentication failed: no username in token", http.StatusBadRequest)
		return
	}

	// Extract groups from configured claim
	var groups []string
	if groupsClaim, ok := claims[h.cfg.OIDCGroupsClaim]; ok {
		switch g := groupsClaim.(type) {
		case []any:
			for _, v := range g {
				if s, ok := v.(string); ok {
					groups = append(groups, s)
				}
			}
		case string:
			groups = []string{g}
		}
	}

	user := &User{Username: username, Groups: groups}

	// Create session cookie (include raw ID token for RP-Initiated Logout)
	secure := true // OIDC typically behind TLS
	http.SetCookie(w, CreateSessionCookieWithIDToken(user, rawIDToken, h.cfg.Secret, h.cfg.CookieTTL, secure))

	log.Printf("[oidc] User %s authenticated (groups: %v)", username, groups)

	// Redirect to app
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout clears the local session and, when the IdP supports RP-Initiated
// Logout, returns a JSON response with a "redirectTo" field containing the IdP's
// end_session_endpoint URL so the frontend can redirect the browser to terminate
// the SSO session.
func (h *OIDCHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Extract ID token before clearing the cookie (needed as id_token_hint)
	idToken := IDTokenFromCookie(r, h.cfg.Secret)

	http.SetCookie(w, ClearSessionCookie())

	resp := map[string]string{"status": "logged out"}

	if h.endSessionEndpoint != "" {
		logoutURL, err := url.Parse(h.endSessionEndpoint)
		if err != nil {
			log.Printf("[oidc] Failed to parse end_session_endpoint %q: %v", h.endSessionEndpoint, err)
		} else {
			q := logoutURL.Query()
			if idToken != "" {
				q.Set("id_token_hint", idToken)
			} else {
				// Fallback for old sessions without stored ID token
				q.Set("client_id", h.cfg.OIDCClientID)
			}
			if h.cfg.OIDCPostLogoutRedirectURL != "" {
				q.Set("post_logout_redirect_uri", h.cfg.OIDCPostLogoutRedirectURL)
			}
			logoutURL.RawQuery = q.Encode()
			resp["redirectTo"] = logoutURL.String()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
