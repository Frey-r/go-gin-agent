package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables.
// Fail-fast: the application panics at boot if any critical secret is missing.
type Config struct {
	// Server
	Port               string
	GinMode            string // "release" | "debug"
	MaxRequestBodySize int64  // bytes

	// JWT
	JWTSecret        string
	JWTRefreshSecret string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration

	// Auth
	BcryptCost             int
	MaxLoginAttempts       int
	LockoutDurationMinutes int

	// CORS
	AllowedOrigins []string

	// SQLite
	SQLitePath string

	// LLM
	GeminiAPIKey     string
	GrokAPIKey       string
	DefaultLLMModel  string // e.g. "gemini-2.5-pro", "grok-3"
	LLMTimeoutSeconds int

	// Langfuse (observability)
	LangfusePublicKey string
	LangfuseSecretKey string
	LangfuseHost      string

	// Rate Limiting
	RateLimitRPM      int // requests per minute (global)
	AuthRateLimitRPM  int // requests per minute (auth endpoints)
}

// Load reads configuration from environment variables and validates required fields.
func Load() *Config {
	cfg := &Config{
		Port:               envOrDefault("PORT", "8080"),
		GinMode:            envOrDefault("GIN_MODE", "release"),
		MaxRequestBodySize: envOrDefaultInt64("MAX_REQUEST_BODY_SIZE", 1<<20), // 1MB

		JWTSecret:        mustEnv("JWT_SECRET"),
		JWTRefreshSecret: mustEnv("JWT_REFRESH_SECRET"),
		AccessTokenTTL:   time.Duration(envOrDefaultInt("ACCESS_TOKEN_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL:  time.Duration(envOrDefaultInt("REFRESH_TOKEN_TTL_DAYS", 7)) * 24 * time.Hour,

		BcryptCost:             envOrDefaultInt("BCRYPT_COST", 12),
		MaxLoginAttempts:       envOrDefaultInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDurationMinutes: envOrDefaultInt("LOCKOUT_DURATION_MINUTES", 15),

		AllowedOrigins: strings.Split(envOrDefault("ALLOWED_ORIGINS", "http://localhost:5173"), ","),

		SQLitePath: envOrDefault("SQLITE_PATH", "data/agent.db"),

		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GrokAPIKey:        os.Getenv("GROK_API_KEY"),
		DefaultLLMModel:   envOrDefault("DEFAULT_LLM_MODEL", "gemini-2.5-pro"),
		LLMTimeoutSeconds: envOrDefaultInt("LLM_TIMEOUT_SECONDS", 120),

		LangfusePublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		LangfuseSecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
		LangfuseHost:      envOrDefault("LANGFUSE_HOST", "https://cloud.langfuse.com"),

		RateLimitRPM:     envOrDefaultInt("RATE_LIMIT_RPM", 60),
		AuthRateLimitRPM: envOrDefaultInt("AUTH_RATE_LIMIT_RPM", 5),
	}

	// Validate at least one LLM provider is configured
	if cfg.GeminiAPIKey == "" && cfg.GrokAPIKey == "" {
		panic("FATAL: at least one LLM API key must be set (GEMINI_API_KEY or GROK_API_KEY)")
	}

	return cfg
}

// mustEnv reads an env var or panics if not set (fail-fast for critical secrets).
func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic(fmt.Sprintf("FATAL: required environment variable %s is not set", key))
	}
	return val
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		n, err := strconv.Atoi(val)
		if err != nil {
			panic(fmt.Sprintf("FATAL: env var %s must be an integer, got %q", key, val))
		}
		return n
	}
	return fallback
}

func envOrDefaultInt64(key string, fallback int64) int64 {
	if val := os.Getenv(key); val != "" {
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			panic(fmt.Sprintf("FATAL: env var %s must be an integer, got %q", key, val))
		}
		return n
	}
	return fallback
}
