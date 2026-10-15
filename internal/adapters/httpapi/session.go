package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookie = "iacpack_session"
	csrfHeader    = "X-Iacpack-CSRF"
	sessionTTL    = 12 * time.Hour
)

type session struct {
	CSRF      string
	ExpiresAt time.Time
}

type sessionStore struct {
	mu   sync.Mutex
	byID map[string]session
}

func newSessionStore() *sessionStore {
	return &sessionStore{byID: map[string]session{}}
}

func (s *sessionStore) create() (id, csrf string, err error) {
	id, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	s.mu.Lock()
	s.byID[id] = session{CSRF: csrf, ExpiresAt: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	return id, csrf, nil
}

func (s *sessionStore) valid(id, csrf string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byID[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return false
	}
	return sess.CSRF == csrf
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func setSessionCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // loopback http
	})
}

func readSessionID(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}
