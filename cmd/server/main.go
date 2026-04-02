package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ebachmann/go-gin-agent/internal/config"
	"github.com/ebachmann/go-gin-agent/internal/handler"
	"github.com/ebachmann/go-gin-agent/internal/llm"
	"github.com/ebachmann/go-gin-agent/internal/middleware"
	"github.com/ebachmann/go-gin-agent/internal/service"
	"github.com/ebachmann/go-gin-agent/internal/store"
	"github.com/ebachmann/go-gin-agent/internal/tools"
)

func main() {
	// ── Structured Logging ──────────────────────────────────────
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("GIN_MODE") != "release" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Msg("starting go-gin-agent orchestrator")

	// ── Configuration ───────────────────────────────────────────
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	// ── Database ────────────────────────────────────────────────
	db, err := store.New(cfg.SQLitePath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize database")
	}
	defer db.Close()

	userStore := store.NewUserStore(db)
	convStore := store.NewConversationStore(db)

	// ── Services ────────────────────────────────────────────────
	authService := service.NewAuthService(userStore, cfg)

	telemetry := service.NewTelemetryService(
		cfg.LangfusePublicKey,
		cfg.LangfuseSecretKey,
		cfg.LangfuseHost,
	)

	// ── LLM Fabric ──────────────────────────────────────────────
	fabric := llm.NewFabric("gemini") // default provider

	if cfg.GeminiAPIKey != "" {
		gemini := llm.NewGeminiProvider(cfg.GeminiAPIKey, cfg.LLMTimeoutSeconds)
		fabric.RegisterProvider(gemini)
		fabric.RegisterModel("gemini-2.5-pro", "gemini")
		fabric.RegisterModel("gemini-2.5-flash", "gemini")
		log.Info().Msg("registered LLM provider: gemini")
	}

	if cfg.GrokAPIKey != "" {
		grok := llm.NewGrokProvider(cfg.GrokAPIKey, cfg.LLMTimeoutSeconds)
		fabric.RegisterProvider(grok)
		fabric.RegisterModel("grok-3", "grok")
		fabric.RegisterModel("grok-3-mini", "grok")
		log.Info().Msg("registered LLM provider: grok")
	}

	// ── Tool Registry ───────────────────────────────────────────
	toolRegistry := tools.NewRegistry()
	// Register tools as needed:
	// toolRegistry.Register("search_knowledge", tools.NewMCPClient("https://mcp.example.com", 30))
	// toolRegistry.Register("run_script", tools.NewLambdaExecutor("https://lambda.example.com", 30))

	// ── Orchestrator ────────────────────────────────────────────
	orchestrator := service.NewOrchestrator(fabric, toolRegistry, convStore, telemetry)

	// ── Handlers ────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authService)
	chatHandler := handler.NewChatHandler(orchestrator, cfg.LLMTimeoutSeconds)
	healthHandler := handler.NewHealthHandler(db)
	webhookHandler := handler.NewWebhookHandler(os.Getenv("WEBHOOK_SECRET"))

	// ── Rate Limiters ───────────────────────────────────────────
	apiLimiter := middleware.NewRateLimiter(cfg.RateLimitRPM)
	authLimiter := middleware.NewRateLimiter(cfg.AuthRateLimitRPM)

	// ── Router ──────────────────────────────────────────────────
	router := gin.New() // no default middleware

	// Global middleware stack (order matters!)
	router.Use(
		middleware.Recovery(),   // catch panics first
		middleware.RequestID(),  // generate trace ID
		middleware.Logger(),     // log request with trace ID
		middleware.Security(),   // security headers
		middleware.CORS(cfg.AllowedOrigins),
	)

	// Limit request body size
	router.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxRequestBodySize)
		c.Next()
	})

	// ── Routes ──────────────────────────────────────────────────

	// Public: Health
	router.GET("/health", healthHandler.Check)

	api := router.Group("/api/v1")

	// Public: Auth (rate-limited by IP)
	auth := api.Group("/auth")
	auth.Use(authLimiter.ByIP())
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
	}

	// Protected: requires valid JWT
	protected := api.Group("")
	protected.Use(middleware.Auth(cfg.JWTSecret))
	protected.Use(apiLimiter.ByTenant())
	{
		protected.POST("/chat/stream", chatHandler.Stream)
		protected.POST("/webhooks", webhookHandler.Handle)
	}

	// ── HTTP Server with Graceful Shutdown ───────────────────────
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // disabled for SSE streaming
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("port", cfg.Port).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	// ── Graceful Shutdown ───────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced shutdown")
	}

	log.Info().Msg("server stopped")
}
