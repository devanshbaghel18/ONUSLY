package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/config"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type mockAuthRepository struct {
	mu           sync.Mutex
	users        map[string]*User
	usersByEmail map[string]*User
	reservations map[string]string // cleanHandle -> userID
	failOnHandle string            // test hook to simulate conditional check failure
}

func newMockAuthRepo() *mockAuthRepository {
	return &mockAuthRepository{
		users:        make(map[string]*User),
		usersByEmail: make(map[string]*User),
		reservations: make(map[string]string),
	}
}

func (m *mockAuthRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (m *mockAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := strings.ToLower(strings.TrimSpace(email))
	u, ok := m.usersByEmail[clean]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (m *mockAuthRepository) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if clean == "" {
		return nil, nil
	}

	// Fail closed: Direct lookup on reservation only, no full table scan
	if uid, ok := m.reservations[clean]; ok {
		if u, found := m.users[uid]; found {
			cp := *u
			return &cp, nil
		}
	}

	return nil, nil
}

func (m *mockAuthRepository) CreateUser(ctx context.Context, user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cleanHandle := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(user.Handle)), "@")
	if cleanHandle != "" {
		if _, exists := m.reservations[cleanHandle]; exists {
			return errors.New("this handle is already taken, please choose another")
		}
		m.reservations[cleanHandle] = user.ID
	}
	cp := *user
	m.users[user.ID] = &cp
	m.usersByEmail[strings.ToLower(strings.TrimSpace(user.Email))] = &cp
	return nil
}

func (m *mockAuthRepository) UpdateUserHandle(ctx context.Context, userID, oldHandle, newHandle string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanNew := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(newHandle)), "@")
	cleanOld := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(oldHandle)), "@")

	if cleanNew == cleanOld {
		return nil
	}

	if m.failOnHandle == cleanNew {
		return errors.New("this handle is already taken, please choose another")
	}

	// Atomic condition: attribute_not_exists(PK) OR UserID = :uid
	if existingOwner, exists := m.reservations[cleanNew]; exists && existingOwner != userID {
		return errors.New("this handle is already taken, please choose another")
	}

	// Reserve new handle
	m.reservations[cleanNew] = userID

	// Free old handle if present and changed
	if cleanOld != "" && cleanOld != cleanNew {
		if m.reservations[cleanOld] == userID {
			delete(m.reservations, cleanOld)
		}
	}

	// Update user record
	if u, exists := m.users[userID]; exists {
		u.Handle = cleanNew
	}

	return nil
}

func TestUpdateUserHandle_Success(t *testing.T) {
	repo := newMockAuthRepo()
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-1",
		Email:  "alice@example.com",
		Handle: "alice_initial",
		Name:   "Alice",
	})

	svc := NewService(repo, config.Config{JWTSecret: "test"})

	profile, err := svc.UpdateUserHandle(context.Background(), "user-1", "alice_new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.Handle != "alice_new" {
		t.Fatalf("expected handle 'alice_new', got '%s'", profile.Handle)
	}

	// Verify old handle was freed and new handle is reserved
	if _, oldExists := repo.reservations["alice_initial"]; oldExists {
		t.Fatalf("expected old handle reservation to be released")
	}
	if repo.reservations["alice_new"] != "user-1" {
		t.Fatalf("expected new handle reserved for user-1")
	}
}

func TestUpdateUserHandle_DuplicateCollision_ReturnsAlreadyTaken(t *testing.T) {
	repo := newMockAuthRepo()
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-bob",
		Email:  "bob@example.com",
		Handle: "master_coder",
		Name:   "Bob",
	})
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-eve",
		Email:  "eve@example.com",
		Handle: "eve_original",
		Name:   "Eve",
	})

	svc := NewService(repo, config.Config{JWTSecret: "test"})

	_, err := svc.UpdateUserHandle(context.Background(), "user-eve", "master_coder")
	if err == nil {
		t.Fatalf("expected error for duplicate handle, got nil")
	}

	if !strings.Contains(err.Error(), "this handle is already taken, please choose another") {
		t.Fatalf("expected 'this handle is already taken, please choose another', got: %v", err)
	}
}

func TestUpdateUserHandle_SimultaneousClaims_OneWinsOneFails(t *testing.T) {
	repo := newMockAuthRepo()
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-1",
		Email:  "u1@example.com",
		Handle: "handle_one",
	})
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-2",
		Email:  "u2@example.com",
		Handle: "handle_two",
	})

	svc := NewService(repo, config.Config{JWTSecret: "test"})

	const targetHandle = "shared_handle"
	var wg sync.WaitGroup
	var results = make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := svc.UpdateUserHandle(context.Background(), "user-1", targetHandle)
		results[0] = err
	}()
	go func() {
		defer wg.Done()
		_, err := svc.UpdateUserHandle(context.Background(), "user-2", targetHandle)
		results[1] = err
	}()

	wg.Wait()

	successes := 0
	failures := 0
	for _, res := range results {
		if res == nil {
			successes++
		} else if strings.Contains(res.Error(), "this handle is already taken, please choose another") {
			failures++
		}
	}

	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly 1 success and 1 conflict failure, got %d successes and %d failures (results: %v)",
			successes, failures, results)
	}
}

func TestUpdateUserHandle_SameHandle_Idempotent(t *testing.T) {
	repo := newMockAuthRepo()
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-1",
		Email:  "u1@example.com",
		Handle: "my_handle",
	})

	svc := NewService(repo, config.Config{JWTSecret: "test"})

	profile, err := svc.UpdateUserHandle(context.Background(), "user-1", "my_handle")
	if err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
	if profile.Handle != "my_handle" {
		t.Fatalf("expected handle 'my_handle', got '%s'", profile.Handle)
	}
}

func TestUpdateUserHandle_InvalidCharactersOrLength(t *testing.T) {
	repo := newMockAuthRepo()
	svc := NewService(repo, config.Config{JWTSecret: "test"})

	cases := []struct {
		handle      string
		expectedErr string
	}{
		{"ab", "handle must be between 3 and 20 characters"},
		{"a_very_long_handle_that_exceeds_twenty_characters", "handle must be between 3 and 20 characters"},
		{"bad-dash", "handle can only contain lowercase letters, numbers, and underscores"},
		{"bad space", "handle can only contain lowercase letters, numbers, and underscores"},
		{"bad!symbol", "handle can only contain lowercase letters, numbers, and underscores"},
	}

	for _, c := range cases {
		_, err := svc.UpdateUserHandle(context.Background(), "user-1", c.handle)
		if err == nil || !strings.Contains(err.Error(), c.expectedErr) {
			t.Errorf("for handle %q expected %q, got: %v", c.handle, c.expectedErr, err)
		}
	}
}

func TestUpdateHandle_HandlerReturns400OnDuplicate(t *testing.T) {
	repo := newMockAuthRepo()
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-bob",
		Email:  "bob@example.com",
		Handle: "taken_handle",
	})
	_ = repo.CreateUser(context.Background(), &User{
		ID:     "user-alice",
		Email:  "alice@example.com",
		Handle: "alice_handle",
	})

	svc := NewService(repo, config.Config{JWTSecret: "test"})
	handler := NewHandler(svc)

	body := strings.NewReader(`{"handle":"taken_handle"}`)
	req := httptest.NewRequest("PATCH", "/users/handle", body)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-alice"))
	rec := httptest.NewRecorder()

	handler.UpdateHandle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "this handle is already taken, please choose another") {
		t.Fatalf("expected duplicate error message in body, got: %s", rec.Body.String())
	}
}

func TestUpdateUserHandle_LegacyUserWithoutReservationCanRename(t *testing.T) {
	repo := newMockAuthRepo()
	// Simulate legacy user: profile exists with handle "legacy_alice", but NO entry in reservations map
	legacyUser := &User{
		ID:     "user-legacy-1",
		Email:  "legacy@example.com",
		Handle: "legacy_alice",
	}
	repo.users[legacyUser.ID] = legacyUser
	repo.usersByEmail[legacyUser.Email] = legacyUser
	// Explicitly ensure no reservation exists for legacy handle
	delete(repo.reservations, "legacy_alice")

	svc := NewService(repo, config.Config{JWTSecret: "test"})

	profile, err := svc.UpdateUserHandle(context.Background(), "user-legacy-1", "alice_modernized")
	if err != nil {
		t.Fatalf("expected legacy user rename to succeed, got error: %v", err)
	}
	if profile.Handle != "alice_modernized" {
		t.Fatalf("expected updated handle 'alice_modernized', got %s", profile.Handle)
	}
	if repo.reservations["alice_modernized"] != "user-legacy-1" {
		t.Fatalf("expected reservation created for modernized handle, got: %v", repo.reservations)
	}
}

func TestGetUserByHandle_ReservationOnlyAndFailsClosed(t *testing.T) {
	repo := newMockAuthRepo()
	user := &User{
		ID:     "user-100",
		Email:  "charlie@example.com",
		Handle: "charlie",
	}
	_ = repo.CreateUser(context.Background(), user)

	// Valid handle with reservation resolves cleanly
	found, err := repo.GetUserByHandle(context.Background(), "@charlie")
	if err != nil {
		t.Fatalf("unexpected error resolving handle: %v", err)
	}
	if found == nil || found.ID != "user-100" {
		t.Fatalf("expected user-100, got %+v", found)
	}

	// Unknown handle without reservation fails closed (returns nil, nil without scan)
	unknown, err := repo.GetUserByHandle(context.Background(), "@ghost_user")
	if err != nil {
		t.Fatalf("unexpected error on unknown handle: %v", err)
	}
	if unknown != nil {
		t.Fatalf("expected nil for unknown handle without reservation, got %+v", unknown)
	}
}
