package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type updateUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

func (s *Server) handleListUsers(c *gin.Context) {
	users, err := s.userService.List(c.Request.Context())
	if err != nil {
		s.logger.Error("list users failed", "error", err)
		Fail(c, http.StatusInternalServerError, "user_error", "failed to list users")
		return
	}
	OK(c, users)
}

func (s *Server) handleCreateUser(c *gin.Context) {
	var request createUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_user", "invalid user payload")
		return
	}
	user, err := s.userService.Create(c.Request.Context(), services.CreateUserInput{
		Username:    request.Username,
		DisplayName: request.DisplayName,
		Email:       request.Email,
		Password:    request.Password,
		Role:        request.Role,
	})
	if err != nil {
		s.handleUserServiceError(c, err)
		return
	}
	Created(c, user)
}

func (s *Server) handleUpdateUser(c *gin.Context) {
	var request updateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_user", "invalid user payload")
		return
	}
	user, err := s.userService.Update(c.Request.Context(), c.Param("userID"), services.UpdateUserInput{
		Username:    request.Username,
		DisplayName: request.DisplayName,
		Email:       request.Email,
		Password:    request.Password,
		Role:        request.Role,
	})
	if err != nil {
		s.handleUserServiceError(c, err)
		return
	}
	OK(c, user)
}

func (s *Server) handleDeleteUser(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	if err := s.userService.Delete(c.Request.Context(), c.Param("userID"), currentUser.ID); err != nil {
		s.handleUserServiceError(c, err)
		return
	}
	OK(c, gin.H{"deleted": true})
}

func (s *Server) handleUserServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidUserInput), errors.Is(err, services.ErrCannotDeleteSelf):
		Fail(c, http.StatusBadRequest, "invalid_user", err.Error())
	case errors.Is(err, services.ErrUserAlreadyExists):
		Fail(c, http.StatusConflict, "user_exists", "username or email already exists")
	case errors.Is(err, services.ErrLastActiveAdmin):
		Fail(c, http.StatusConflict, "last_admin", "at least one active admin is required")
	case errors.Is(err, services.ErrUserNotFound):
		Fail(c, http.StatusNotFound, "user_not_found", "user not found")
	default:
		s.logger.Error("user administration failed", "error", err)
		Fail(c, http.StatusInternalServerError, "user_error", "user operation failed")
	}
}
