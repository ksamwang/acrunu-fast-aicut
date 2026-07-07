package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token string    `json:"token"`
	User  auth.User `json:"user"`
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

	if req.Username != s.cfg.AdminUsername || req.Password != s.cfg.AdminPassword {
		Fail(c, http.StatusUnauthorized, "unauthorized", "invalid username or password")
		return
	}

	user := auth.User{
		ID:          "dev-admin",
		Username:    s.cfg.AdminUsername,
		DisplayName: "Admin",
		Role:        auth.RoleAdmin,
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
