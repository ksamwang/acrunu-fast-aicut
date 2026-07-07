package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string    `json:"token"`
	User  auth.User `json:"user"`
}

type upsertSystemConfigRequest struct {
	Value       any    `json:"value" binding:"required"`
	Type        string `json:"type"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description"`
}

func (s *Server) handleHealth(c *gin.Context) {
	OK(c, gin.H{
		"status": "ok",
		"env":    s.cfg.AppEnv,
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid login payload")
		return
	}

	user, err := s.userService.Login(req.Username, req.Password)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "unauthorized", "invalid username or password")
		return
	}

	OK(c, loginResponse{
		Token: makeDevToken(user),
		User:  user,
	})
}

func (s *Server) handleMe(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	OK(c, user)
}

func (s *Server) handleListSystemConfigs(c *gin.Context) {
	OK(c, s.systemConfigService.List())
}

func (s *Server) handleSystemConfigSnapshot(c *gin.Context) {
	OK(c, s.systemConfigService.Snapshot())
}

func (s *Server) handleUpsertSystemConfig(c *gin.Context) {
	var req upsertSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid system config payload")
		return
	}

	key := c.Param("key")
	if key == "" {
		Fail(c, http.StatusBadRequest, "bad_request", "missing config key")
		return
	}

	existing, err := s.systemConfigService.Get(key)
	if err != nil && !errors.Is(err, services.ErrConfigNotFound) {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to read system config")
		return
	}

	configType := req.Type
	if configType == "" {
		configType = existing.Type
	}
	if configType == "" {
		configType = "json"
	}

	config := s.systemConfigService.Upsert(services.SystemConfig{
		Key:         key,
		Value:       req.Value,
		Type:        configType,
		IsSecret:    req.IsSecret,
		Description: req.Description,
	})

	OK(c, config)
}
