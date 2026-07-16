package httpserver

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
)

const requestIDHeader = "X-Request-ID"

func (s *Server) requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set("request_id", requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)
		c.Next()
	}
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		s.logger.Info(
			"http request",
			slog.String("request_id", c.GetString("request_id")),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(start)),
		)
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			Fail(c, http.StatusUnauthorized, "unauthorized", "missing authorization header")
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" || token == header {
			Fail(c, http.StatusUnauthorized, "unauthorized", "invalid authorization header")
			return
		}

		user, err := s.userService.Authenticate(c.Request.Context(), token)
		if err != nil {
			Fail(c, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}

		auth.SetUser(c, user)
		c.Next()
	}
}

func (s *Server) requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
			return
		}
		if string(user.Role) != role {
			Fail(c, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}

		c.Next()
	}
}
