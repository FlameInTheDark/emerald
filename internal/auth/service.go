package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

const (
	DefaultCookieName = "emerald_session"
	DefaultSessionTTL = 24 * time.Hour
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
}

type Config struct {
	SessionTTL time.Duration
	CookieName string
}

type Session struct {
	UserID    string
	Username  string
	ExpiresAt time.Time
}

type Service struct {
	users      UserStore
	sessionTTL time.Duration
	cookieName string

	now func() time.Time

	mu       sync.Mutex
	sessions map[string]Session
}

func NewService(users UserStore, cfg Config) *Service {
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = DefaultSessionTTL
	}

	cookieName := strings.TrimSpace(cfg.CookieName)
	if cookieName == "" {
		cookieName = DefaultCookieName
	}

	return &Service{
		users:      users,
		sessionTTL: sessionTTL,
		cookieName: cookieName,
		now:        time.Now,
		sessions:   make(map[string]Session),
	}
}

func (s *Service) CookieName() string {
	return s.cookieName
}

func (s *Service) Authenticate(ctx context.Context, username string, password string) (*models.User, error) {
	return s.authenticateUser(ctx, username, password)
}

func (s *Service) Login(ctx context.Context, username string, password string) (string, Session, error) {
	user, err := s.authenticateUser(ctx, username, password)
	if err != nil {
		return "", Session{}, err
	}

	token, err := generateToken()
	if err != nil {
		return "", Session{}, err
	}

	session := Session{
		UserID:    user.ID,
		Username:  user.Username,
		ExpiresAt: s.now().Add(s.sessionTTL),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[token] = session
	return token, session, nil
}

func (s *Service) authenticateUser(ctx context.Context, username string, password string) (*models.User, error) {
	if s.users == nil {
		return nil, errors.New("user store is not configured")
	}

	user, err := s.users.GetByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return nil, err
	}
	if user == nil || !secureMatch(password, user.Password) {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

func (s *Service) Session(token string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sessionLocked(token)
}

func (s *Service) Logout(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)
}

func (s *Service) RevokeUserSessions(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for token, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, token)
		}
	}
}

func (s *Service) sessionLocked(token string) (Session, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, false
	}

	session, ok := s.sessions[token]
	if !ok {
		return Session{}, false
	}

	if !session.ExpiresAt.After(s.now()) {
		delete(s.sessions, token)
		return Session{}, false
	}

	return session, true
}

func secureMatch(got string, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
