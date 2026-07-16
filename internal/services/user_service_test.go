package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
)

func TestUserServiceDefaultAdminAndSessions(t *testing.T) {
	service := NewUserService(config.Config{})
	admin, err := service.Login(context.Background(), "admin", "admin123")
	if err != nil {
		t.Fatalf("login default admin: %v", err)
	}
	if admin.Role != auth.RoleAdmin || admin.ID == "" {
		t.Fatalf("unexpected default admin %#v", admin)
	}

	token, err := service.CreateSession(admin)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	resolved, err := service.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate admin session: %v", err)
	}
	if resolved.ID != admin.ID || resolved.Role != auth.RoleAdmin {
		t.Fatalf("unexpected authenticated user %#v", resolved)
	}
}

func TestUserServiceManagesUsersAndProtectsLastAdmin(t *testing.T) {
	service := NewUserService(config.Config{})
	admin, err := service.Login(context.Background(), "admin", "admin123")
	if err != nil {
		t.Fatalf("login default admin: %v", err)
	}

	user, err := service.Create(context.Background(), CreateUserInput{
		Username:    "editor",
		DisplayName: "剪辑用户",
		Password:    "editor123",
		Role:        "user",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Role != auth.RoleUser {
		t.Fatalf("unexpected user role %#v", user)
	}

	updated, err := service.Update(context.Background(), user.ID, UpdateUserInput{
		Username:    "editor",
		DisplayName: "编辑用户",
		Password:    "updated123",
		Role:        "admin",
	})
	if err != nil {
		t.Fatalf("promote user: %v", err)
	}
	if updated.Role != auth.RoleAdmin {
		t.Fatalf("unexpected updated role %#v", updated)
	}

	if err := service.Delete(context.Background(), admin.ID, admin.ID); !errors.Is(err, ErrCannotDeleteSelf) {
		t.Fatalf("expected self-delete protection, got %v", err)
	}
	if err := service.Delete(context.Background(), updated.ID, admin.ID); err != nil {
		t.Fatalf("delete second admin: %v", err)
	}
	if err := service.Delete(context.Background(), admin.ID, "another-admin"); !errors.Is(err, ErrLastActiveAdmin) {
		t.Fatalf("expected last admin protection, got %v", err)
	}

	if _, err := service.Login(context.Background(), "editor", "updated123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deleted user should not log in, got %v", err)
	}
}
