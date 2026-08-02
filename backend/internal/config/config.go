package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig
	Postgres  PostgresConfig
	Groq      GroqConfig
	Models    ModelsConfig
	AI        AIConfig
	Log       LogConfig
	Fabric    FabricConfig
	Providers ProvidersConfig
	Redis     RedisConfig
}

// ProvidersConfig selects the bank / KYC / SIM integration provider. Each
// defaults to "mock" (offline demo). Set to a real provider name to flip the
// integration by config only (BANK_PROVIDER=razorpay, KYC_PROVIDER=nsdl,
// SIM_PROVIDER=telecom). An unknown value falls back to mock with a warning.
type ProvidersConfig struct {
	Bank string
	KYC  string
	SIM  string
}

type RedisConfig struct {
	URL      string // e.g. redis://localhost:6379
	Password string
	DB       int
}

// FabricConfig points the backend at the local Hyperledger Fabric network that
// backs the FINIX event ledger. When Enabled is false the service keeps using
// the legacy in-memory ledger, so the app runs with or without the network.
type FabricConfig struct {
	Enabled      bool
	PeerEndpoint string // host:port of the gateway peer (e.g. localhost:7051)
	GatewayPeer  string // TLS server-name override (e.g. peer0.finix.local)
	Channel      string
	Chaincode    string
	MSPID        string
	// CryptoPath is the peerOrganizations/<org> dir produced by cryptogen; the
	// cert/key/TLS paths below default to the standard layout underneath it.
	CryptoPath  string
	CertPath    string
	KeyDir      string
	TLSCertPath string
}

type ServerConfig struct {
	Addr                string
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	ShutdownTimeout     time.Duration
	AllowedOrigins      []string
	RateLimitPerMinute  int
	DisableCORSWildcard bool
}

type PostgresConfig struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
	SSLMode  string
	MaxConns int32
}

type GroqConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type ModelsConfig struct {
	// RiskModelPath is the transaction-risk ONNX classifier.
	// MuleModelPath is the mule-account GNN whose RecipientGNNScore
	// feeds the risk model — see MODEL_IO_CONTRACT.md.
	RiskModelPath string
	MuleModelPath string
}

// AIConfig configures the AI ecosystem integration (Python FINIX RAG service).
type AIConfig struct {
	Provider      string // "local" | "remote" — controls whether Go calls local Groq or proxies to Python RAG
	RagBaseURL    string // e.g. http://finix-rag:8000 (internal Docker DNS)
	InternalToken string // FINIX_INTERNAL_TOKEN for service-to-service auth
	ReadOnlyDSN   string // Postgres read-replica DSN for RAG validation worker (postgres://user:pass@host:5432/db)
}

type LogConfig struct {
	Level  string
	Format string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:                envOrDefault("APP_ADDR", "0.0.0.0:8080"),
			ReadTimeout:         envDuration("APP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:        envDuration("APP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:         envDuration("APP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:     envDuration("APP_SHUTDOWN_TIMEOUT", 10*time.Second),
			AllowedOrigins:      envList("CORS_ALLOWED_ORIGINS", []string{}),
			RateLimitPerMinute:  envInt("RATE_LIMIT_PER_MINUTE", 60),
			DisableCORSWildcard: strings.EqualFold(os.Getenv("CORS_DISABLE_WILDCARD"), "true"),
		},
		Postgres: PostgresConfig{
			Host:     envOrDefault("PGHOST", ""),
			Port:     envOrDefault("PGPORT", "5432"),
			Database: envOrDefault("PGDATABASE", "finix"),
			User:     envOrDefault("PGUSER", "finix"),
			Password: envOrDefault("PGPASSWORD", ""),
			SSLMode:  envOrDefault("PGSSLMODE", "disable"),
			MaxConns: int32(envInt("PG_MAX_CONNS", 20)),
		},
		Groq: GroqConfig{
			APIKey:  os.Getenv("GROQ_API_KEY"),
			BaseURL: envOrDefault("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
			// llama3-8b-8192 was the previous default but Groq has decommissioned
			// it — the API rejects it, the chat call errors, and the service
			// silently falls back to template replies. Default to a model that
			// is actually served; override with GROQ_MODEL.
			Model: envOrDefault("GROQ_MODEL", "llama-3.3-70b-versatile"),
		},
		Models: ModelsConfig{
			RiskModelPath: envOrDefault("FINIX_RISK_MODEL_PATH", "../../models/transaction_risk_model.onnx"),
			MuleModelPath: envOrDefault("FINIX_MULE_MODEL_PATH", "../../models/MuleAccountDetection.onnx"),
		},
		AI: AIConfig{
			Provider:      envOrDefault("AI_PROVIDER", "local"),
			RagBaseURL:    envOrDefault("AIML_UPSTREAM_URL", "http://localhost:8000"),
			InternalToken: os.Getenv("FINIX_INTERNAL_TOKEN"),
			ReadOnlyDSN:   os.Getenv("RAG_PG_DSN"),
		},
		Log: LogConfig{
			Level:  envOrDefault("LOG_LEVEL", "info"),
			Format: envOrDefault("LOG_FORMAT", "text"),
		},
		Fabric: loadFabric(),
		Providers: ProvidersConfig{
			Bank: strings.ToLower(envOrDefault("BANK_PROVIDER", "mock")),
			KYC:  strings.ToLower(envOrDefault("KYC_PROVIDER", "mock")),
			SIM:  strings.ToLower(envOrDefault("SIM_PROVIDER", "mock")),
		},
		Redis: RedisConfig{
			URL:      envOrDefault("REDIS_URL", "redis://localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       envInt("REDIS_DB", 0),
		},
	}
}

// loadFabric reads the FABRIC_* env. Paths default to the cryptogen layout under
// FABRIC_CRYPTO_PATH so a normal local run only needs FABRIC_ENABLED=true.
func loadFabric() FabricConfig {
	cryptoPath := envOrDefault("FABRIC_CRYPTO_PATH",
		"../../deploy/fabric/organizations/peerOrganizations/finix.local")
	return FabricConfig{
		Enabled:      strings.EqualFold(os.Getenv("FABRIC_ENABLED"), "true"),
		PeerEndpoint: envOrDefault("FABRIC_PEER_ENDPOINT", "localhost:7051"),
		GatewayPeer:  envOrDefault("FABRIC_GATEWAY_PEER", "peer0.finix.local"),
		Channel:      envOrDefault("FABRIC_CHANNEL", "finix-channel"),
		Chaincode:    envOrDefault("FABRIC_CHAINCODE", "finix"),
		MSPID:        envOrDefault("FABRIC_MSP_ID", "FinixMSP"),
		CryptoPath:   cryptoPath,
		CertPath: envOrDefault("FABRIC_CERT_PATH",
			cryptoPath+"/users/User1@finix.local/msp/signcerts"),
		KeyDir: envOrDefault("FABRIC_KEY_DIR",
			cryptoPath+"/users/User1@finix.local/msp/keystore"),
		TLSCertPath: envOrDefault("FABRIC_TLS_CERT_PATH",
			cryptoPath+"/peers/peer0.finix.local/tls/ca.crt"),
	}
}

func (c *Config) Validate() error {
	var errs []string

	if c.Server.Addr == "" {
		errs = append(errs, "APP_ADDR must not be empty")
	}
	if c.Server.ReadTimeout <= 0 {
		errs = append(errs, "APP_READ_TIMEOUT must be positive")
	}
	if c.Server.WriteTimeout <= 0 {
		errs = append(errs, "APP_WRITE_TIMEOUT must be positive")
	}

	if c.Postgres.Host != "" && c.Postgres.Database == "" {
		errs = append(errs, "PGDATABASE must be set when PGHOST is set")
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.Log.Level)] {
		errs = append(errs, fmt.Sprintf("LOG_LEVEL must be one of: debug, info, warn, error (got: %s)", c.Log.Level))
	}

	if c.Groq.APIKey == "" && os.Getenv("GROQ_API_KEY") == "" {
		errs = append(errs, "GROQ_API_KEY must be set for chatbot functionality (use '«redacted:sk-…»' to disable)")
	}

	if c.Server.RateLimitPerMinute <= 0 {
		errs = append(errs, "RATE_LIMIT_PER_MINUTE must be positive")
	}

	// Sprint 8: Startup config validation - require critical secrets in production
	if !isDevEnv() {
		if os.Getenv("FINIX_JWT_SECRET") == "" {
			errs = append(errs, "FINIX_JWT_SECRET must be set (>= 16 chars)")
		}
		if c.AI.Provider == "remote" {
			if c.AI.RagBaseURL == "" {
				errs = append(errs, "AIML_UPSTREAM_URL must be set when AI_PROVIDER=remote")
			}
			if c.AI.InternalToken == "" {
				errs = append(errs, "FINIX_INTERNAL_TOKEN must be set when AI_PROVIDER=remote")
			}
		}
		if os.Getenv("FINIX_INTERNAL_TOKEN") == "" {
			errs = append(errs, "FINIX_INTERNAL_TOKEN must be set for service-to-service auth")
		}
		if c.Redis.URL != "" && os.Getenv("REDIS_PASSWORD") == "" {
			// Warn but don't fail - Redis can work without password
			slog.Warn("REDIS_PASSWORD not set - Redis connection will be unauthenticated")
		}
		if os.Getenv("FINIX_REVOCATION_FAIL_CLOSED") != "true" {
			slog.Warn("FINIX_REVOCATION_FAIL_CLOSED not set to true - JWT revocation uses fail-open policy")
		}
		if os.Getenv("CERT_PINS") == "" {
			slog.Warn("CERT_PINS not set - certificate pinning is disabled")
		}
		if os.Getenv("FINIX_PIN_ENFORCE") != "true" {
			slog.Warn("FINIX_PIN_ENFORCE not set to true - certificate pinning enforcement is disabled")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func isDevEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_ENV"))) {
	case "dev", "development", "local", "test":
		return true
	default:
		return false
	}
}

func (c *Config) PostgresDSN() string {
	if c.Postgres.Host == "" {
		return ""
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&pool_max_conns=%d",
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.Database,
		c.Postgres.SSLMode,
		c.Postgres.MaxConns,
	)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil && i > 0 {
			return i
		}
	}
	return fallback
}

func envList(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
