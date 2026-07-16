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

	user, err := s.userService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "unauthorized", "invalid username or password")
		return
	}
	token, err := s.userService.CreateSession(user)
	if err != nil {
		s.logger.Error("create login session failed", "error", err)
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to create login session")
		return
	}

	OK(c, loginResponse{
		Token: token,
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
	configs, err := s.systemConfigService.List()
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to list system configs")
		return
	}
	OK(c, configs)
}

func (s *Server) handleSystemConfigSnapshot(c *gin.Context) {
	snapshot, err := s.systemConfigService.Snapshot()
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to read system config snapshot")
		return
	}
	OK(c, snapshot)
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

	config, err := s.systemConfigService.Upsert(services.SystemConfig{
		Key:         key,
		Value:       req.Value,
		Type:        configType,
		IsSecret:    req.IsSecret,
		Description: req.Description,
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to save system config")
		return
	}

	OK(c, config)
}
