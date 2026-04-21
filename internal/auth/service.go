package auth

import (
	"context"
	"crypto/rand"
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
var ErrRateLimited = errors.New("too many login attempts")

type UserStore interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	UpdatePassword(ctx context.Context, id string, password string) error
}

type LegacyPasswordStore interface {
	VerifyLegacyPassword(stored string, provided string) (bool, error)
}

type SessionStore interface {
	Create(ctx context.Context, token string, session Session) error
	GetByToken(ctx context.Context, token string, now time.Time) (Session, bool, error)
	Delete(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

type Config struct {
	SessionTTL   time.Duration
	CookieName   string
	SessionStore SessionStore
}

type Session struct {
	UserID       string
	Username     string
	IsSuperAdmin bool
	ExpiresAt    time.Time
}

type Service struct {
	users      UserStore
	sessions   SessionStore
	sessionTTL time.Duration
	cookieName string

	now func() time.Time

	throttle *loginThrottle
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
		sessions:   newSessionStore(cfg.SessionStore),
		sessionTTL: sessionTTL,
		cookieName: cookieName,
		now:        time.Now,
		throttle:   newLoginThrottle(),
	}
}

func (s *Service) CookieName() string {
	return s.cookieName
}

func (s *Service) Authenticate(ctx context.Context, username string, password string) (*models.User, error) {
	return s.authenticateUser(ctx, username, password)
}

func (s *Service) Login(ctx context.Context, username string, password string) (string, Session, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	if s.throttle.Blocked(key, s.now()) {
		return "", Session{}, ErrRateLimited
	}

	user, err := s.authenticateUser(ctx, username, password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			s.throttle.Failed(key, s.now())
		}
		return "", Session{}, err
	}
	s.throttle.Succeeded(key)

	token, err := generateToken()
	if err != nil {
		return "", Session{}, err
	}

	session := Session{
		UserID:       user.ID,
		Username:     user.Username,
		IsSuperAdmin: user.IsSuperAdmin,
		ExpiresAt:    s.now().Add(s.sessionTTL),
	}

	if err := s.sessions.Create(ctx, token, session); err != nil {
		return "", Session{}, err
	}
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
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if IsPasswordHash(user.Password) {
		match, err := VerifyPasswordHash(user.Password, password)
		if err != nil {
			return nil, err
		}
		if !match {
			return nil, ErrInvalidCredentials
		}
		return user, nil
	}

	legacyStore, ok := s.users.(LegacyPasswordStore)
	if !ok {
		return nil, ErrInvalidCredentials
	}
	match, err := legacyStore.VerifyLegacyPassword(user.Password, password)
	if err != nil {
		return nil, err
	}
	if !match {
		return nil, ErrInvalidCredentials
	}
	if err := s.users.UpdatePassword(ctx, user.ID, password); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) Session(token string) (Session, bool) {
	session, ok, err := s.sessions.GetByToken(context.Background(), token, s.now())
	if err != nil {
		return Session{}, false
	}
	return session, ok
}

func (s *Service) Logout(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	_ = s.sessions.Delete(context.Background(), token)
}

func (s *Service) RevokeUserSessions(userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	_ = s.sessions.DeleteByUserID(context.Background(), userID)
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

type loginThrottle struct {
	mu       sync.Mutex
	entries  map[string]loginThrottleEntry
	limit    int
	duration time.Duration
}

type loginThrottleEntry struct {
	failures     int
	blockedUntil time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{
		entries:  make(map[string]loginThrottleEntry),
		limit:    5,
		duration: time.Minute,
	}
}

func (l *loginThrottle) Blocked(key string, now time.Time) bool {
	if l == nil || key == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		return false
	}
	if entry.blockedUntil.After(now) {
		return true
	}
	if !entry.blockedUntil.IsZero() {
		delete(l.entries, key)
	}
	return false
}

func (l *loginThrottle) Failed(key string, now time.Time) {
	if l == nil || key == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[key]
	entry.failures++
	if entry.failures >= l.limit {
		entry.blockedUntil = now.Add(l.duration)
		entry.failures = 0
	}
	l.entries[key] = entry
}

func (l *loginThrottle) Succeeded(key string) {
	if l == nil || key == "" {
		return
	}

	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func newSessionStore(store SessionStore) SessionStore {
	if store != nil {
		return store
	}
	return &memorySessionStore{
		sessions: make(map[string]Session),
	}
}

type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func (s *memorySessionStore) Create(_ context.Context, token string, session Session) error {
	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()
	return nil
}

func (s *memorySessionStore) GetByToken(_ context.Context, token string, now time.Time) (Session, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[token]
	if !ok {
		return Session{}, false, nil
	}
	if !session.ExpiresAt.After(now) {
		delete(s.sessions, token)
		return Session{}, false, nil
	}
	return session, true, nil
}

func (s *memorySessionStore) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	delete(s.sessions, strings.TrimSpace(token))
	s.mu.Unlock()
	return nil
}

func (s *memorySessionStore) DeleteByUserID(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, session := range s.sessions {
		if session.UserID == strings.TrimSpace(userID) {
			delete(s.sessions, token)
		}
	}
	return nil
}
