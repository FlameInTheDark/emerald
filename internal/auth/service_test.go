package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type stubUserStore struct {
	user           *models.User
	err            error
	legacyPassword string
}

func (s *stubUserStore) GetByUsername(_ context.Context, username string) (*models.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.user != nil && s.user.Username == username {
		return s.user, nil
	}
	return nil, nil
}

func (s *stubUserStore) UpdatePassword(_ context.Context, id string, password string) error {
	if s.user == nil || s.user.ID != id {
		return nil
	}
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}
	s.user.Password = hashed
	return nil
}

func (s *stubUserStore) VerifyLegacyPassword(stored string, provided string) (bool, error) {
	return s.legacyPassword != "" && stored == s.user.Password && provided == s.legacyPassword, nil
}

func TestServiceLoginAndSession(t *testing.T) {
	t.Parallel()

	service := NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: hashedPasswordForTest(t, "admin")},
	}, Config{})

	token, session, err := service.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}
	if session.Username != "admin" {
		t.Fatalf("session username = %q, want admin", session.Username)
	}
	if session.UserID != "user-1" {
		t.Fatalf("session userID = %q, want user-1", session.UserID)
	}

	stored, ok := service.Session(token)
	if !ok {
		t.Fatal("expected session lookup to succeed")
	}
	if stored.Username != session.Username {
		t.Fatalf("stored username = %q, want %q", stored.Username, session.Username)
	}
}

func TestServiceRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	service := NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: hashedPasswordForTest(t, "admin")},
	}, Config{})

	_, _, err := service.Login(context.Background(), "admin", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestServiceAuthenticateReturnsUser(t *testing.T) {
	t.Parallel()

	service := NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: hashedPasswordForTest(t, "admin")},
	}, Config{})

	user, err := service.Authenticate(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.ID != "user-1" {
		t.Fatalf("user ID = %q, want user-1", user.ID)
	}
}

func TestServiceExpiresSessions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 8, 12, 0, 0, 0, time.UTC)
	service := NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: hashedPasswordForTest(t, "admin")},
	}, Config{SessionTTL: time.Minute})
	service.now = func() time.Time { return now }

	token, _, err := service.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	now = now.Add(2 * time.Minute)

	if _, ok := service.Session(token); ok {
		t.Fatal("expected expired session lookup to fail")
	}
}

func TestServiceMigratesLegacyPasswords(t *testing.T) {
	t.Parallel()

	store := &stubUserStore{
		user:           &models.User{ID: "user-1", Username: "admin", Password: "legacy-ciphertext"},
		legacyPassword: "admin",
	}
	service := NewService(store, Config{})

	token, _, err := service.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}
	if !IsPasswordHash(store.user.Password) {
		t.Fatalf("password = %q, want argon2id hash", store.user.Password)
	}
	match, err := VerifyPasswordHash(store.user.Password, "admin")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !match {
		t.Fatal("expected migrated password hash to verify")
	}
}

func hashedPasswordForTest(t *testing.T, password string) string {
	t.Helper()

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return hashed
}
