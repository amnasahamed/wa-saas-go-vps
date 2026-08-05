// Package config loads runtime configuration from environment variables.
// Single source of truth for all service-level settings.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings for both cmd/api and cmd/bridge.
// Values are populated from environment variables in Load().
type Config struct {
	// Service identity
	ServiceName string // "api" or "bridge"
	Env         string // "dev" | "staging" | "prod"
	LogLevel    string // "debug" | "info" | "warn" | "error"

	// HTTP/gRPC
	APIListenAddr  string // e.g. ":8080" for cmd/api
	BridgeGRPCAddr string // e.g. ":9090" for cmd/bridge

	// gRPC service-to-service (Phase 1). The api and bridge communicate over
	// these listeners/targets on the private compose network. The two listeners
	// were originally the same port, which silently dialed gRPC over the API's
	// HTTP port; split them.
	APIGRPCListenAddr string // e.g. ":8081" for the cmd/api gRPC server
	APIGRPCTarget     string // e.g. "api:8081" — bridge-to-api client target
	BridgeGRPCTargets map[string]string // worker_id -> "bridge:9090"

	// Service-to-service auth. Used as the `x-api-key` metadata value on every
	// gRPC call in both directions. Required outside dev; tolerated in dev so
	// local docker-compose.dev.yml still boots without extra config.
	GRPCSharedSecret string

	// Postgres
	DatabaseURL string // postgres://user:pass@host:5432/dbname?sslmode=require

	// Redis
	RedisURL string // redis://host:6379/0

	// S3 / object storage
	S3Region      string
	S3MediaBucket string
	S3AccessKey   string
	S3SecretKey   string
	S3Endpoint    string // optional, for LocalStack in dev

	// AWS
	AWSRegion   string
	AWSKMSKeyID string // per-tenant secrets encryption

	// WhatsApp (bridge only)
	BridgeWorkerID    string // unique per bridge instance, used for session sharding
	BridgeMaxSessions int    // soft cap on whatsmeow clients per process
	BridgeStateDir    string // durable dir for per-session SQLite device stores

	// OpenAI
	OpenAIAPIKey  string // platform-level key; tenants can BYO
	OpenAIModel   string // default "gpt-4o"
	OpenAIBaseURL string // for testing

	// Stripe (api only)
	StripeSecretKey     string
	StripeWebhookSecret string

	// Inbound webhook auth
	InboundAPIBaseURL string // public URL of cmd/api (for QR code links)

	// AWS SES (api only)
	SESFromEmail string // e.g. "no-reply@niosengine.app"

	// Timeouts
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	GRPCDialTimeout     time.Duration
	ShutdownGracePeriod time.Duration
}

// Load reads configuration from the environment.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:         getEnv("SERVICE_NAME", "api"),
		Env:                 getEnv("ENV", "dev"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		APIListenAddr:       getEnv("API_LISTEN_ADDR", ":8080"),
		BridgeGRPCAddr:      getEnv("BRIDGE_GRPC_ADDR", ":9090"),
		APIGRPCListenAddr:   getEnv("API_GRPC_LISTEN_ADDR", ":8081"),
		APIGRPCTarget:       getEnv("API_GRPC_ADDR", "api:8081"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		S3Region:            getEnv("AWS_REGION", "us-east-1"),
		S3MediaBucket:       os.Getenv("S3_MEDIA_BUCKET"),
		S3AccessKey:         os.Getenv("AWS_ACCESS_KEY_ID"),
		S3SecretKey:         os.Getenv("AWS_SECRET_ACCESS_KEY"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		AWSRegion:           getEnv("AWS_REGION", "us-east-1"),
		AWSKMSKeyID:         os.Getenv("AWS_KMS_KEY_ID"),
		BridgeWorkerID:      os.Getenv("BRIDGE_WORKER_ID"),
		BridgeMaxSessions:   getEnvInt("BRIDGE_MAX_SESSIONS", 50),
		BridgeStateDir:      getEnv("BRIDGE_STATE_DIR", "/var/lib/niosengine/bridge"),
		OpenAIAPIKey:        os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-4o"),
		OpenAIBaseURL:       getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		InboundAPIBaseURL:   getEnv("INBOUND_API_BASE_URL", "http://localhost:8080"),
		SESFromEmail:        os.Getenv("SES_FROM_EMAIL"),
		HTTPReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 30*time.Second),
		HTTPWriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 60*time.Second),
		GRPCDialTimeout:     getEnvDuration("GRPC_DIAL_TIMEOUT", 5*time.Second),
		ShutdownGracePeriod: getEnvDuration("SHUTDOWN_GRACE_PERIOD", 30*time.Second),
		GRPCSharedSecret:    os.Getenv("GRPC_SHARED_SECRET"),
		BridgeGRPCTargets:   parseWorkerTargets(getEnv("BRIDGE_GRPC_TARGETS", "worker-1=bridge:9090")),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate ensures required env vars are present. Tolerates missing optional
// values that aren't needed in the current service mode.
func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	// Redis is required once Phase 1 lands the reply delay queue.
	// For now we tolerate empty REDIS_URL so the api can boot in dev.
	if c.RedisURL == "" && !c.IsDev() {
		return fmt.Errorf("REDIS_URL is required outside dev")
	}
	// Service-to-service gRPC auth. Required outside dev; tolerated in dev
	// so local docker-compose.dev.yml still boots without extra config.
	if c.GRPCSharedSecret == "" && !c.IsDev() {
		return fmt.Errorf("GRPC_SHARED_SECRET is required outside dev " +
			"(generate one with: openssl rand -base64 48)")
	}
	// Service-specific required vars are validated at the call site
	// (e.g. cmd/bridge requires BRIDGE_WORKER_ID, cmd/api requires STRIPE_*).
	return nil
}

// IsDev returns true when running in local dev mode.
func (c *Config) IsDev() bool { return c.Env == "dev" }

// IsProd returns true when running in production.
func (c *Config) IsProd() bool { return c.Env == "prod" }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// parseWorkerTargets parses `worker-1=bridge:9090,worker-2=bridge2:9090` into a
// map. Empty input yields a single-entry default (worker-1=bridge:9090) so the
// single-worker default works without env wiring. Each entry is validated.
func parseWorkerTargets(raw string) map[string]string {
	out := make(map[string]string)
	for _, pair := range splitAndTrim(raw, ",") {
		eq := strings.IndexByte(pair, '=')
		if eq <= 0 || eq == len(pair)-1 {
			// Malformed entries are skipped silently — the orchestrator logs
			// which targets it loaded at startup, so a typo is visible.
			continue
		}
		id := strings.TrimSpace(pair[:eq])
		target := strings.TrimSpace(pair[eq+1:])
		if id != "" && target != "" {
			out[id] = target
		}
	}
	if len(out) == 0 {
		out["worker-1"] = "bridge:9090"
	}
	return out
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
