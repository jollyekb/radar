package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// DefaultCookieName is the default session cookie name
const DefaultCookieName = "radar_session"

// cookiePayload is the data stored in the session cookie
type cookiePayload struct {
	Username  string   `json:"u"`
	Groups    []string `json:"g,omitempty"`
	ExpiresAt int64    `json:"e"`
	IDToken   string   `json:"t,omitempty"` // raw OIDC id_token for RP-Initiated Logout
}

// CreateSessionCookie creates a signed session cookie for the given user.
// Format: base64(json) + "." + base64(hmac-sha256)
func CreateSessionCookie(user *User, secret string, ttl time.Duration, secure bool) *http.Cookie {
	return CreateSessionCookieWithIDToken(user, "", secret, ttl, secure)
}

// ParseSessionCookie validates and parses a session cookie.
// Returns nil if the cookie is missing, invalid, or expired.
func ParseSessionCookie(r *http.Request, secret string) *User {
	cookie, err := r.Cookie(DefaultCookieName)
	if err != nil {
		return nil
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	encoded, sig := parts[0], parts[1]

	// Verify HMAC signature
	expected := signData(encoded, secret)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		log.Printf("[auth] Session cookie HMAC verification failed — possible tampered cookie from %s", r.RemoteAddr)
		return nil
	}

	// Decode payload
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}

	var p cookiePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil
	}

	// Check expiration
	if time.Now().Unix() > p.ExpiresAt {
		log.Printf("[auth] Session cookie expired for user %q — prompting re-auth", p.Username)
		return nil
	}

	return &User{
		Username: p.Username,
		Groups:   p.Groups,
	}
}

// CreateSessionCookieWithIDToken creates a signed session cookie that also stores
// the raw OIDC ID token for use as id_token_hint during RP-Initiated Logout.
func CreateSessionCookieWithIDToken(user *User, idToken string, secret string, ttl time.Duration, secure bool) *http.Cookie {
	payload := cookiePayload{
		Username:  user.Username,
		Groups:    user.Groups,
		ExpiresAt: time.Now().Add(ttl).Unix(),
		IDToken:   idToken,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("[auth] Failed to marshal session cookie payload for user %s: %v", user.Username, err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)

	sig := signData(encoded, secret)

	return &http.Cookie{
		Name:     DefaultCookieName,
		Value:    encoded + "." + sig,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	}
}

// IDTokenFromCookie extracts the raw OIDC ID token from the session cookie.
// Returns "" if the cookie is missing, invalid, expired, or has no ID token.
func IDTokenFromCookie(r *http.Request, secret string) string {
	cookie, err := r.Cookie(DefaultCookieName)
	if err != nil {
		return ""
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return ""
	}

	encoded, sig := parts[0], parts[1]

	expected := signData(encoded, secret)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return ""
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}

	var p cookiePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}

	if time.Now().Unix() > p.ExpiresAt {
		return ""
	}

	return p.IDToken
}

// ClearSessionCookie returns a cookie that clears the session
func ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     DefaultCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	}
}

// signData computes HMAC-SHA256 of the given data with the secret
func signData(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprint(mac, data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
