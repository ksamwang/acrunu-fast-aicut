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
	Config                     config.Config
	Logger                     *slog.Logger
	UserService                *services.UserService
	TaskService                *services.TaskService
	SystemConfigService        *services.SystemConfigService
	ModelProviderService       *services.ModelProviderService
	ProductAssetService        *services.ProductAssetService
	AssetEmbeddingService      *services.AssetEmbeddingService
	VoiceoverService           *services.VoiceoverService
	ScriptGenerationService    *services.ScriptGenerationService
	GenerationRunService       *services.GenerationRunService
	SubtitleStylePresetService *services.SubtitleStylePresetService
	BGMTrackService            *services.BGMTrackService
}

type Server struct {
	cfg                        config.Config
	logger                     *slog.Logger
	engine                     *gin.Engine
	userService                *services.UserService
	systemConfigService        *services.SystemConfigService
	modelProviderService       *services.ModelProviderService
	productAssetService        *services.ProductAssetService
	assetEmbeddingService      *services.AssetEmbeddingService
	voiceoverService           *services.VoiceoverService
	scriptGenerationService    *services.ScriptGenerationService
	generationRunService       *services.GenerationRunService
	subtitleStylePresetService *services.SubtitleStylePresetService
	bgmTrackService            *services.BGMTrackService
	uploadTokenService         *services.UploadTokenService
	localStore                 *storage.LocalStore
	taskService                *services.TaskService
	queueClient                *queue.Client
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
	modelProviderService := opts.ModelProviderService
	if modelProviderService == nil {
		modelProviderService = services.NewModelProviderService()
	}

	productAssetService := opts.ProductAssetService
	if productAssetService == nil {
		productAssetService = services.NewProductAssetService()
	}
	assetEmbeddingService := opts.AssetEmbeddingService
	if assetEmbeddingService == nil {
		assetEmbeddingService = services.NewAssetEmbeddingService(nil, productAssetService, systemConfigService, modelProviderService, opts.Config)
	}
	voiceoverService := opts.VoiceoverService
	if voiceoverService == nil {
		voiceoverService = services.NewVoiceoverService(opts.Config.StorageRoot, opts.Config, opts.Logger)
	}
	scriptGenerationService := opts.ScriptGenerationService
	if scriptGenerationService == nil {
		scriptGenerationService = services.NewScriptGenerationService(productAssetService, systemConfigService, modelProviderService, opts.Config)
	}
	generationRunService := opts.GenerationRunService
	if generationRunService == nil {
		generationRunService = services.NewGenerationRunService(voiceoverService)
	}
	subtitleStylePresetService := opts.SubtitleStylePresetService
	if subtitleStylePresetService == nil {
		subtitleStylePresetService = services.NewSubtitleStylePresetService()
	}
	bgmTrackService := opts.BGMTrackService
	if bgmTrackService == nil {
		bgmTrackService = services.NewBGMTrackService(opts.Config.StorageRoot)
	}

	userService := opts.UserService
	if userService == nil {
		userService = services.NewUserService(opts.Config)
	}

	server := &Server{
		cfg:                        opts.Config,
		logger:                     opts.Logger,
		engine:                     gin.New(),
		userService:                userService,
		systemConfigService:        systemConfigService,
		modelProviderService:       modelProviderService,
		productAssetService:        productAssetService,
		assetEmbeddingService:      assetEmbeddingService,
		voiceoverService:           voiceoverService,
		scriptGenerationService:    scriptGenerationService,
		generationRunService:       generationRunService,
		subtitleStylePresetService: subtitleStylePresetService,
		bgmTrackService:            bgmTrackService,
		uploadTokenService:         services.NewUploadTokenService(),
		localStore:                 storage.NewLocalStore(opts.Config.StorageRoot),
		taskService:                taskService,
		queueClient:                queue.NewClient(opts.Config.RedisAddr, opts.Config.QueueBackend, opts.Config.StorageRoot),
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

	users := adminGroup.Group("/users")
	users.GET("", s.handleListUsers)
	users.POST("", s.handleCreateUser)
	users.PUT("/:userID", s.handleUpdateUser)
	users.DELETE("/:userID", s.handleDeleteUser)

	systemConfigs := adminGroup.Group("/system-configs")
	systemConfigs.GET("", s.handleListSystemConfigs)
	systemConfigs.GET("/snapshot", s.handleSystemConfigSnapshot)
	systemConfigs.PUT("/:key", s.handleUpsertSystemConfig)

	modelAccess := adminGroup.Group("/model-access/openai-compatible")
	modelAccess.GET("", s.handleGetOpenAICompatibleSettings)
	modelAccess.PUT("", s.handleUpdateOpenAICompatibleSettings)
	modelAccess.POST("/test", s.handleTestOpenAICompatibleConnection)
	modelAccess.POST("/models", s.handleListOpenAICompatibleModels)

	modelProviders := adminGroup.Group("/model-providers")
	modelProviders.GET("", s.handleListModelProviders)
	modelProviders.POST("", s.handleCreateModelProvider)
	modelProviders.PUT("/:providerID", s.handleUpdateModelProvider)
	modelProviders.DELETE("/:providerID", s.handleDeleteModelProvider)
	modelProviders.POST("/:providerID/test", s.handleTestModelProvider)
	modelProviders.POST("/:providerID/models", s.handleListModelProviderModels)

	modelSettings := adminGroup.Group("/model-settings")
	modelSettings.GET("", s.handleGetModelCapabilitySettings)
	modelSettings.PUT("", s.handleUpdateModelCapabilitySettings)

	runtimeSettings := adminGroup.Group("/runtime-settings")
	runtimeSettings.GET("", s.handleGetRuntimeSettings)
	runtimeSettings.PUT("", s.handleUpdateRuntimeSettings)

	voiceProfileAdmin := adminGroup.Group("/voice-profiles")
	voiceProfileAdmin.GET("", s.handleListVoiceProfiles)
	voiceProfileAdmin.POST("", s.handleCreateVoiceProfile)
	voiceProfileAdmin.PUT("/:voiceProfileID", s.handleUpdateVoiceProfile)
	voiceProfileAdmin.POST("/:voiceProfileID/default", s.handleSetDefaultVoiceProfile)
	voiceProfileAdmin.DELETE("/:voiceProfileID", s.handleDeleteVoiceProfile)

	subtitleStylesAdmin := adminGroup.Group("/subtitle-presets")
	subtitleStylesAdmin.GET("", s.handleAdminListSubtitleStylePresets)
	subtitleStylesAdmin.POST("", s.handleCreateSubtitleStylePreset)
	subtitleStylesAdmin.PUT("/:presetID", s.handleUpdateSubtitleStylePreset)
	subtitleStylesAdmin.POST("/:presetID/default", s.handleSetDefaultSubtitleStylePreset)
	subtitleStylesAdmin.DELETE("/:presetID", s.handleDeleteSubtitleStylePreset)

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
	protected.DELETE("/products/:productID", s.handleDeleteProduct)
	protected.POST("/products/:productID/archive", s.handleArchiveProduct)
	protected.POST("/products/:productID/selling-points", s.handleCreateSellingPoint)
	protected.GET("/products/:productID/selling-points", s.handleListSellingPoints)
	protected.GET("/selling-points/:sellingPointID/assets", s.handleListSellingPointAssets)
	protected.PUT("/selling-points/:sellingPointID", s.handleUpdateSellingPoint)
	protected.DELETE("/selling-points/:sellingPointID", s.handleDeleteSellingPoint)
	protected.POST("/selling-points/:sellingPointID/archive", s.handleArchiveSellingPoint)
	protected.POST("/uploads/tokens", s.handleCreateUploadToken)
	protected.POST("/preprocess/asr-transcribe", s.handlePreprocessASRTranscribe)
	protected.POST("/preprocess/vlm-label", s.handlePreprocessVLMLabel)
	protected.POST("/workbench/scripts/generate", s.handleGenerateWorkbenchScripts)
	protected.GET("/voice-profiles", s.handleListVoiceProfiles)
	protected.GET("/subtitle-presets", s.handleListSubtitleStylePresets)
	protected.GET("/bgm-tracks", s.handleListBGMTracks)
	protected.POST("/bgm-tracks", s.handleCreateBGMTrack)
	protected.PUT("/bgm-tracks/:trackID", s.handleUpdateBGMTrack)
	protected.DELETE("/bgm-tracks/:trackID", s.handleArchiveBGMTrack)
	protected.POST("/voice-profiles/:voiceProfileID/auditions", s.handleCreateVoiceAudition)
	protected.GET("/voice-auditions/:auditionID", s.handleGetVoiceAudition)
	protected.POST("/workbench/voiceover-tasks", s.handleCreateVoiceoverTasks)
	protected.GET("/workbench/works", s.handleListVoiceoverWorks)
	protected.GET("/workbench/works/:taskID", s.handleGetVoiceoverWork)
	protected.POST("/workbench/works/:taskID/retry", s.handleRetryVoiceoverWork)
	protected.POST("/workbench/works/:taskID/regenerate", s.handleRegenerateVoiceoverWork)
	protected.DELETE("/workbench/works/:taskID", s.handleDeleteVoiceoverWork)
	protected.GET("/assets", s.handleListAssets)
	protected.GET("/assets/:assetID", s.handleGetAsset)
	protected.GET("/assets/:assetID/frames", s.handleListAssetFrames)
	protected.GET("/assets/:assetID/selling-points", s.handleListAssetSellingPoints)
	protected.GET("/assets/:assetID/speech-segments", s.handleListAssetSpeechSegments)
	protected.GET("/assets/:assetID/semantic-preview", s.handleGetAssetSemanticPreview)
	protected.GET("/assets/:assetID/embeddings", s.handleListAssetEmbeddings)
	protected.POST("/assets/:assetID/embeddings", s.handleVectorizeAsset)
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
