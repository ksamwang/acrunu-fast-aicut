package services

import (
	"errors"

	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type UserService struct {
	adminUsername string
	adminPassword string
}

func NewUserService(cfg config.Config) *UserService {
	return &UserService{
		adminUsername: cfg.AdminUsername,
		adminPassword: cfg.AdminPassword,
	}
}

func (s *UserService) Login(username string, password string) (auth.User, error) {
	if username != s.adminUsername || password != s.adminPassword {
		return auth.User{}, ErrInvalidCredentials
	}

	return auth.User{
		ID:          "dev-admin",
		Username:    s.adminUsername,
		DisplayName: "Admin",
		Role:        auth.RoleAdmin,
	}, nil
}
