package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserInactive       = errors.New("user is inactive")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidUserInput   = errors.New("invalid user input")
	ErrCannotDeleteSelf   = errors.New("cannot delete current user")
	ErrLastActiveAdmin    = errors.New("cannot remove the last active admin")
)

const (
	defaultAdminUsername = "admin"
	defaultAdminPassword = "admin123"
)

type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email,omitempty"`
	Role        auth.Role  `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateUserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type UpdateUserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type memoryUser struct {
	user         User
	passwordHash string
}

// UserService owns account administration and process-local login sessions.
// Persistent deployments always resolve a session against PostgreSQL so status
// and role changes take effect on the next request.
type UserService struct {
	queries *db.Queries

	adminMu     sync.Mutex
	mu          sync.RWMutex
	sessions    map[string]string
	memoryUsers map[string]memoryUser
}

func NewUserService(_ config.Config) *UserService {
	now := time.Now().UTC()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.MinCost)
	if err != nil {
		panic(fmt.Sprintf("hash default admin password: %v", err))
	}
	adminID := uuid.NewString()
	return &UserService{
		sessions: map[string]string{},
		memoryUsers: map[string]memoryUser{
			adminID: {
				user: User{
					ID:          adminID,
					Username:    defaultAdminUsername,
					DisplayName: "管理员",
					Role:        auth.RoleAdmin,
					Status:      "active",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				passwordHash: string(passwordHash),
			},
		},
	}
}

func NewUserServiceWithQueries(queries *db.Queries) *UserService {
	if queries == nil {
		return NewUserService(config.Config{})
	}
	return &UserService{
		queries:  queries,
		sessions: map[string]string{},
	}
}

func (s *UserService) Login(ctx context.Context, username string, password string) (auth.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return auth.User{}, ErrInvalidCredentials
	}

	if s.queries == nil {
		return s.loginMemory(username, password)
	}

	row, err := s.queries.GetUserByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return auth.User{}, err
	}
	if row.Status != "active" {
		return auth.User{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		return auth.User{}, ErrInvalidCredentials
	}
	if err := s.queries.UpdateUserLastLogin(ctx, row.ID); err != nil {
		return auth.User{}, err
	}
	return authUserFromDB(row)
}

func (s *UserService) CreateSession(user auth.User) (string, error) {
	if strings.TrimSpace(user.ID) == "" {
		return "", ErrInvalidSession
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	s.mu.Lock()
	s.sessions[token] = user.ID
	s.mu.Unlock()
	return token, nil
}

func (s *UserService) Authenticate(ctx context.Context, token string) (auth.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return auth.User{}, ErrInvalidSession
	}

	s.mu.RLock()
	userID, ok := s.sessions[token]
	s.mu.RUnlock()
	if ok {
		return s.GetAuthUser(ctx, userID)
	}

	// Legacy encoded tokens keep the in-memory test server lightweight. They
	// are never accepted by a PostgreSQL-backed server.
	if s.queries == nil {
		if user, valid := parseLegacyUserToken(token); valid {
			return user, nil
		}
	}
	return auth.User{}, ErrInvalidSession
}

func (s *UserService) GetAuthUser(ctx context.Context, userID string) (auth.User, error) {
	if s.queries == nil {
		s.mu.RLock()
		stored, ok := s.memoryUsers[userID]
		s.mu.RUnlock()
		if !ok {
			return auth.User{}, ErrUserNotFound
		}
		if stored.user.Status != "active" {
			return auth.User{}, ErrUserInactive
		}
		return auth.User{
			ID:          stored.user.ID,
			Username:    stored.user.Username,
			DisplayName: stored.user.DisplayName,
			Role:        stored.user.Role,
		}, nil
	}

	id, err := userUUID(userID)
	if err != nil {
		return auth.User{}, ErrUserNotFound
	}
	row, err := s.queries.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, ErrUserNotFound
	}
	if err != nil {
		return auth.User{}, err
	}
	if row.Status != "active" {
		return auth.User{}, ErrUserInactive
	}
	return authUserFromDB(row)
}

func (s *UserService) List(ctx context.Context) ([]User, error) {
	if s.queries == nil {
		s.mu.RLock()
		users := make([]User, 0, len(s.memoryUsers))
		for _, stored := range s.memoryUsers {
			users = append(users, stored.user)
		}
		s.mu.RUnlock()
		sortUsersByCreatedAt(users)
		return users, nil
	}

	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]User, 0, len(rows))
	for _, row := range rows {
		user, err := userFromDB(row)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *UserService) Create(ctx context.Context, input CreateUserInput) (User, error) {
	input, err := normalizeCreateUserInput(input)
	if err != nil {
		return User{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash user password: %w", err)
	}

	if s.queries == nil {
		return s.createMemory(input, string(passwordHash))
	}

	row, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Username:     input.Username,
		DisplayName:  input.DisplayName,
		Email:        nullableText(input.Email),
		PasswordHash: string(passwordHash),
		Role:         input.Role,
		Status:       "active",
	})
	if err != nil {
		return User{}, normalizeUserStorageError(err)
	}
	return userFromDB(row)
}

func (s *UserService) Update(ctx context.Context, targetUserID string, input UpdateUserInput) (User, error) {
	input, err := normalizeUpdateUserInput(input)
	if err != nil {
		return User{}, err
	}

	if s.queries == nil {
		return s.updateMemory(targetUserID, input)
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()

	targetID, err := userUUID(targetUserID)
	if err != nil {
		return User{}, ErrUserNotFound
	}
	current, err := s.queries.GetUserByID(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	if current.Role == string(auth.RoleAdmin) && current.Status == "active" && input.Role != string(auth.RoleAdmin) {
		if err := s.ensureAnotherActiveAdmin(ctx); err != nil {
			return User{}, err
		}
	}

	passwordHash := current.PasswordHash
	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, fmt.Errorf("hash user password: %w", err)
		}
		passwordHash = string(hash)
	}
	row, err := s.queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:           targetID,
		Username:     input.Username,
		DisplayName:  input.DisplayName,
		Email:        nullableText(input.Email),
		PasswordHash: passwordHash,
		Role:         input.Role,
	})
	if err != nil {
		return User{}, normalizeUserStorageError(err)
	}
	return userFromDB(row)
}

func (s *UserService) Delete(ctx context.Context, targetUserID string, currentUserID string) error {
	if strings.TrimSpace(targetUserID) == "" {
		return ErrUserNotFound
	}
	if targetUserID == currentUserID {
		return ErrCannotDeleteSelf
	}

	if s.queries == nil {
		return s.deleteMemory(targetUserID)
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()

	targetID, err := userUUID(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}
	current, err := s.queries.GetUserByID(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if current.Role == string(auth.RoleAdmin) && current.Status == "active" {
		if err := s.ensureAnotherActiveAdmin(ctx); err != nil {
			return err
		}
	}
	deleted, err := s.queries.DeleteUser(ctx, targetID)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrUserNotFound
	}
	s.deleteSessionsForUser(targetUserID)
	return nil
}

func (s *UserService) loginMemory(username string, password string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, stored := range s.memoryUsers {
		if stored.user.Username != username {
			continue
		}
		if stored.user.Status != "active" || bcrypt.CompareHashAndPassword([]byte(stored.passwordHash), []byte(password)) != nil {
			return auth.User{}, ErrInvalidCredentials
		}
		now := time.Now().UTC()
		stored.user.LastLoginAt = &now
		stored.user.UpdatedAt = now
		s.memoryUsers[id] = stored
		return auth.User{ID: stored.user.ID, Username: stored.user.Username, DisplayName: stored.user.DisplayName, Role: stored.user.Role}, nil
	}
	return auth.User{}, ErrInvalidCredentials
}

func (s *UserService) createMemory(input CreateUserInput, passwordHash string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stored := range s.memoryUsers {
		if stored.user.Username == input.Username {
			return User{}, ErrUserAlreadyExists
		}
		if input.Email != "" && stored.user.Email == input.Email {
			return User{}, ErrUserAlreadyExists
		}
	}
	now := time.Now().UTC()
	user := User{
		ID:          uuid.NewString(),
		Username:    input.Username,
		DisplayName: input.DisplayName,
		Email:       input.Email,
		Role:        auth.Role(input.Role),
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.memoryUsers[user.ID] = memoryUser{user: user, passwordHash: passwordHash}
	return user, nil
}

func (s *UserService) updateMemory(targetUserID string, input UpdateUserInput) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.memoryUsers[targetUserID]
	if !ok {
		return User{}, ErrUserNotFound
	}
	if stored.user.Role == auth.RoleAdmin && stored.user.Status == "active" && input.Role != string(auth.RoleAdmin) && s.memoryActiveAdminCount() <= 1 {
		return User{}, ErrLastActiveAdmin
	}
	for id, candidate := range s.memoryUsers {
		if id == targetUserID {
			continue
		}
		if candidate.user.Username == input.Username || (input.Email != "" && candidate.user.Email == input.Email) {
			return User{}, ErrUserAlreadyExists
		}
	}
	if input.Password != "" {
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, fmt.Errorf("hash user password: %w", err)
		}
		stored.passwordHash = string(passwordHash)
	}
	stored.user.Username = input.Username
	stored.user.DisplayName = input.DisplayName
	stored.user.Email = input.Email
	stored.user.Role = auth.Role(input.Role)
	stored.user.UpdatedAt = time.Now().UTC()
	s.memoryUsers[targetUserID] = stored
	return stored.user, nil
}

func (s *UserService) deleteMemory(targetUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.memoryUsers[targetUserID]
	if !ok {
		return ErrUserNotFound
	}
	if stored.user.Role == auth.RoleAdmin && stored.user.Status == "active" && s.memoryActiveAdminCount() <= 1 {
		return ErrLastActiveAdmin
	}
	delete(s.memoryUsers, targetUserID)
	for token, userID := range s.sessions {
		if userID == targetUserID {
			delete(s.sessions, token)
		}
	}
	return nil
}

func (s *UserService) memoryActiveAdminCount() int {
	count := 0
	for _, stored := range s.memoryUsers {
		if stored.user.Role == auth.RoleAdmin && stored.user.Status == "active" {
			count++
		}
	}
	return count
}

func (s *UserService) ensureAnotherActiveAdmin(ctx context.Context) error {
	count, err := s.queries.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastActiveAdmin
	}
	return nil
}

func (s *UserService) deleteSessionsForUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sessionUserID := range s.sessions {
		if sessionUserID == userID {
			delete(s.sessions, token)
		}
	}
}

func normalizeCreateUserInput(input CreateUserInput) (CreateUserInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	input.Password = strings.TrimSpace(input.Password)
	input.Role = strings.TrimSpace(input.Role)
	if err := validateUserIdentity(input.Username, input.DisplayName, input.Email, input.Role); err != nil {
		return CreateUserInput{}, err
	}
	if len([]rune(input.Password)) < 6 {
		return CreateUserInput{}, fmt.Errorf("%w: password must be at least 6 characters", ErrInvalidUserInput)
	}
	return input, nil
}

func normalizeUpdateUserInput(input UpdateUserInput) (UpdateUserInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	input.Password = strings.TrimSpace(input.Password)
	input.Role = strings.TrimSpace(input.Role)
	if err := validateUserIdentity(input.Username, input.DisplayName, input.Email, input.Role); err != nil {
		return UpdateUserInput{}, err
	}
	if input.Password != "" && len([]rune(input.Password)) < 6 {
		return UpdateUserInput{}, fmt.Errorf("%w: password must be at least 6 characters", ErrInvalidUserInput)
	}
	return input, nil
}

func validateUserIdentity(username string, displayName string, email string, role string) error {
	if len([]rune(username)) < 2 || len([]rune(username)) > 64 || strings.IndexFunc(username, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%w: username must be 2-64 characters without spaces", ErrInvalidUserInput)
	}
	if len([]rune(displayName)) == 0 || len([]rune(displayName)) > 100 {
		return fmt.Errorf("%w: display_name is required", ErrInvalidUserInput)
	}
	if email != "" && (len([]rune(email)) > 254 || !strings.Contains(email, "@")) {
		return fmt.Errorf("%w: email is invalid", ErrInvalidUserInput)
	}
	if role != string(auth.RoleAdmin) && role != string(auth.RoleUser) {
		return fmt.Errorf("%w: role must be admin or user", ErrInvalidUserInput)
	}
	return nil
}

func userFromDB(row db.User) (User, error) {
	if !row.ID.Valid {
		return User{}, fmt.Errorf("user id is invalid")
	}
	role := auth.Role(row.Role)
	if role != auth.RoleAdmin && role != auth.RoleUser {
		return User{}, fmt.Errorf("user role is invalid")
	}
	user := User{
		ID:          uuid.UUID(row.ID.Bytes).String(),
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Role:        role,
		Status:      row.Status,
	}
	if row.Email.Valid {
		user.Email = row.Email.String
	}
	if row.LastLoginAt.Valid {
		lastLoginAt := row.LastLoginAt.Time
		user.LastLoginAt = &lastLoginAt
	}
	if row.CreatedAt.Valid {
		user.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		user.UpdatedAt = row.UpdatedAt.Time
	}
	return user, nil
}

func authUserFromDB(row db.User) (auth.User, error) {
	user, err := userFromDB(row)
	if err != nil {
		return auth.User{}, err
	}
	return auth.User{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role}, nil
}

func userUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func normalizeUserStorageError(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" {
		return ErrUserAlreadyExists
	}
	return err
}

func parseLegacyUserToken(token string) (auth.User, bool) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return auth.User{}, false
	}
	var user auth.User
	if err := json.Unmarshal(payload, &user); err != nil {
		return auth.User{}, false
	}
	if user.ID == "" || user.Username == "" || (user.Role != auth.RoleAdmin && user.Role != auth.RoleUser) {
		return auth.User{}, false
	}
	return user, true
}

func sortUsersByCreatedAt(users []User) {
	for index := 1; index < len(users); index++ {
		for previous := index; previous > 0 && users[previous].CreatedAt.After(users[previous-1].CreatedAt); previous-- {
			users[previous], users[previous-1] = users[previous-1], users[previous]
		}
	}
}
