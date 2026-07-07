package httpserver

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
)

type Options struct {
	Config config.Config
	Logger *slog.Logger
}

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	engine *gin.Engine
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	server := &Server{
		cfg:    opts.Config,
		logger: opts.Logger,
		engine: gin.New(),
	}

	server.routes()
	return server
}

func (s *Server) Run() error {
	return s.engine.Run(s.cfg.APIAddr)
}

func (s *Server) routes() {
	s.engine.Use(s.requestIDMiddleware())
	s.engine.Use(s.loggingMiddleware())
	s.engine.Use(gin.Recovery())

	api := s.engine.Group("/api")
	api.GET("/healthz", s.handleHealth)

	authGroup := api.Group("/auth")
	authGroup.POST("/login", s.handleLogin)
	authGroup.GET("/me", s.authMiddleware(), s.handleMe)

	adminGroup := api.Group("/admin")
	adminGroup.Use(s.authMiddleware(), s.requireRole("admin"))
	adminGroup.GET("/ping", func(c *gin.Context) {
		OK(c, gin.H{"message": "admin"})
	})

	protected := api.Group("")
	protected.Use(s.authMiddleware())
	protected.GET("/ping", func(c *gin.Context) {
		OK(c, gin.H{"message": "ok"})
	})
}
