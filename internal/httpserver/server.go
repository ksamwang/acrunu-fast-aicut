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
	Config              config.Config
	Logger              *slog.Logger
	TaskService         *services.TaskService
	SystemConfigService *services.SystemConfigService
	ProductAssetService *services.ProductAssetService
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

	systemConfigService := opts.SystemConfigService
	if systemConfigService == nil {
		systemConfigService = services.NewSystemConfigService()
	}

	productAssetService := opts.ProductAssetService
	if productAssetService == nil {
		productAssetService = services.NewProductAssetService()
	}

	server := &Server{
		cfg:                 opts.Config,
		logger:              opts.Logger,
		engine:              gin.New(),
		userService:         services.NewUserService(opts.Config),
		systemConfigService: systemConfigService,
		productAssetService: productAssetService,
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

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) routes() {
	s.engine.Use(s.requestIDMiddleware())
	s.engine.Use(s.loggingMiddleware())
	s.engine.Use(gin.Recovery())

	api := s.engine.Group("/api")
	api.GET("/healthz", s.handleHealth)
	s.engine.GET("/metrics", s.handleMetrics)

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

	modelAccess := adminGroup.Group("/model-access/openai-compatible")
	modelAccess.GET("", s.handleGetOpenAICompatibleSettings)
	modelAccess.PUT("", s.handleUpdateOpenAICompatibleSettings)
	modelAccess.POST("/test", s.handleTestOpenAICompatibleConnection)
	modelAccess.POST("/models", s.handleListOpenAICompatibleModels)

	runtimeSettings := adminGroup.Group("/runtime-settings")
	runtimeSettings.GET("", s.handleGetRuntimeSettings)
	runtimeSettings.PUT("", s.handleUpdateRuntimeSettings)

	protected := api.Group("")
	protected.Use(s.authMiddleware())
	protected.GET("/ping", func(c *gin.Context) {
		OK(c, gin.H{"message": "ok"})
	})
	protected.POST("/products", s.handleCreateProduct)
	protected.GET("/products", s.handleListProducts)
	protected.GET("/products/:productID", s.handleGetProduct)
	protected.GET("/products/:productID/stats", s.handleGetProductStats)
	protected.PUT("/products/:productID", s.handleUpdateProduct)
	protected.POST("/products/:productID/archive", s.handleArchiveProduct)
	protected.POST("/products/:productID/selling-points", s.handleCreateSellingPoint)
	protected.GET("/products/:productID/selling-points", s.handleListSellingPoints)
	protected.GET("/selling-points/:sellingPointID/assets", s.handleListSellingPointAssets)
	protected.PUT("/selling-points/:sellingPointID", s.handleUpdateSellingPoint)
	protected.POST("/selling-points/:sellingPointID/archive", s.handleArchiveSellingPoint)
	protected.POST("/uploads/tokens", s.handleCreateUploadToken)
	protected.POST("/preprocess/vlm-label", s.handlePreprocessVLMLabel)
	protected.GET("/assets", s.handleListAssets)
	protected.GET("/assets/:assetID", s.handleGetAsset)
	protected.GET("/assets/:assetID/frames", s.handleListAssetFrames)
	protected.GET("/assets/:assetID/selling-points", s.handleListAssetSellingPoints)
	protected.GET("/assets/:assetID/speech-segments", s.handleListAssetSpeechSegments)
	protected.GET("/assets/:assetID/semantic-preview", s.handleGetAssetSemanticPreview)
	protected.PUT("/assets/:assetID/review", s.handleUpdateAssetReview)
	protected.PUT("/assets/:assetID/selling-points", s.handleUpdateAssetSellingPoints)
	protected.PUT("/assets/:assetID/business-tags", s.handleUpdateAssetBusinessTags)
	protected.POST("/assets/:assetID/archive", s.handleArchiveAsset)
	protected.POST("/assets/:assetID/restore", s.handleRestoreAsset)
	protected.POST("/tasks/test", s.handleCreateTestTask)
	protected.GET("/tasks", s.handleListTasks)
	protected.GET("/tasks/:taskID", s.handleGetTask)

	api.POST("/uploads/clean-shot", s.handleUploadCleanShot)
	s.engine.Static("/storage", s.cfg.StorageRoot)
}
