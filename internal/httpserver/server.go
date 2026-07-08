package httpserver

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
	"github.com/ksamwang/acrunu-fast-aicut/internal/storage"
)

type Options struct {
	Config      config.Config
	Logger      *slog.Logger
	TaskService *services.TaskService
}

type Server struct {
	cfg                 config.Config
	logger              *slog.Logger
	engine              *gin.Engine
	userService         *services.UserService
	systemConfigService *services.SystemConfigService
	productAssetService *services.ProductAssetService
	uploadTokenService  *services.UploadTokenService
	localStore          *storage.LocalStore
	taskService         *services.TaskService
	queueClient         *queue.Client
}

func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	taskService := opts.TaskService
	if taskService == nil {
		taskService = services.NewTaskService(opts.Config.StorageRoot)
	}

	server := &Server{
		cfg:                 opts.Config,
		logger:              opts.Logger,
		engine:              gin.New(),
		userService:         services.NewUserService(opts.Config),
		systemConfigService: services.NewSystemConfigService(),
		productAssetService: services.NewProductAssetService(),
		uploadTokenService:  services.NewUploadTokenService(),
		localStore:          storage.NewLocalStore(opts.Config.StorageRoot),
		taskService:         taskService,
		queueClient:         queue.NewClient(opts.Config.RedisAddr, opts.Config.QueueBackend, opts.Config.StorageRoot),
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

	systemConfigs := adminGroup.Group("/system-configs")
	systemConfigs.GET("", s.handleListSystemConfigs)
	systemConfigs.GET("/snapshot", s.handleSystemConfigSnapshot)
	systemConfigs.PUT("/:key", s.handleUpsertSystemConfig)

	protected := api.Group("")
	protected.Use(s.authMiddleware())
	protected.GET("/ping", func(c *gin.Context) {
		OK(c, gin.H{"message": "ok"})
	})
	protected.POST("/products", s.handleCreateProduct)
	protected.GET("/products", s.handleListProducts)
	protected.GET("/products/:productID", s.handleGetProduct)
	protected.PUT("/products/:productID", s.handleUpdateProduct)
	protected.POST("/products/:productID/archive", s.handleArchiveProduct)
	protected.POST("/products/:productID/selling-points", s.handleCreateSellingPoint)
	protected.GET("/products/:productID/selling-points", s.handleListSellingPoints)
	protected.PUT("/selling-points/:sellingPointID", s.handleUpdateSellingPoint)
	protected.POST("/selling-points/:sellingPointID/archive", s.handleArchiveSellingPoint)
	protected.POST("/uploads/tokens", s.handleCreateUploadToken)
	protected.GET("/assets", s.handleListAssets)
	protected.GET("/assets/:assetID", s.handleGetAsset)
	protected.POST("/tasks/test", s.handleCreateTestTask)
	protected.GET("/tasks", s.handleListTasks)

	api.POST("/uploads/clean-shot", s.handleUploadCleanShot)
}
