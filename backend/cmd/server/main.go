package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"FINIX/backend/internal/api"
	"FINIX/backend/internal/config"
	"FINIX/backend/internal/domain/platform"
	"FINIX/backend/internal/domain/security"
	"FINIX/backend/internal/infra/ai"
	"FINIX/backend/internal/infra/bank"
	"FINIX/backend/internal/infra/db"
	"FINIX/backend/internal/infra/fabric"
	"FINIX/backend/internal/infra/kyc"
	"FINIX/backend/internal/infra/sim"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	setupLogging(cfg)

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// C-7/C-8: refuse to boot without the mandatory secrets. These live in main
	// (not package init) so unit tests are not killed by a missing env var.
	platform.MustHaveDataEncryptionKey()
	security.MustHaveAadhaarHashKey()

	slog.Info("FINIX backend starting",
		"version", "0.2.0",
		"log_level", cfg.Log.Level,
		"log_format", cfg.Log.Format,
	)

	var dbPool any
	if cfg.Postgres.Host != "" {
		slog.Info("attempting PostgreSQL connection",
			"host", cfg.Postgres.Host,
			"port", cfg.Postgres.Port,
			"database", cfg.Postgres.Database,
		)

		pool, err := db.New(db.Config{
			Host:     cfg.Postgres.Host,
			Port:     cfg.Postgres.Port,
			Database: cfg.Postgres.Database,
			User:     cfg.Postgres.User,
			Password: cfg.Postgres.Password,
			SSLMode:  cfg.Postgres.SSLMode,
		})

		if err != nil {
			slog.Warn("PostgreSQL connection failed, falling back to in-memory storage",
				"error", err,
				"host", cfg.Postgres.Host,
			)
		} else {
			slog.Info("PostgreSQL connected successfully",
				"host", cfg.Postgres.Host,
				"database", cfg.Postgres.Database,
			)

			migrationsDir := os.Getenv("FINIX_MIGRATIONS_DIR")
			if migrationsDir == "" {
				migrationsDir = "migrations"
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := db.Migrate(ctx, pool, migrationsDir); err != nil {
				slog.Warn("migration execution had issues", "error", err)
			} else {
				slog.Info("database migrations applied", "dir", migrationsDir)
			}
			cancel()

			dbPool = pool
		}
	} else {
		slog.Info("no PGHOST set, running with in-memory storage only")
	}

	// ── Hyperledger Fabric event ledger ──────────────────────
	// When enabled, Fabric becomes the system of record: WriteEvent submits a
	// chaincode transaction (smart contracts enforced on-chain) and committed
	// events are mirrored into Postgres for fast API reads. If the network is
	// unreachable we log and fall back to the in-memory ledger rather than
	// refusing to start.
	var routerOpts []api.RouterOption
	var fabricClient *fabric.Client
	ledgerMode := "in-memory"
	syncCtx, stopSync := context.WithCancel(context.Background())
	defer stopSync()

	if cfg.Fabric.Enabled {
		slog.Info("connecting to Hyperledger Fabric",
			"peer", cfg.Fabric.PeerEndpoint,
			"channel", cfg.Fabric.Channel,
			"chaincode", cfg.Fabric.Chaincode,
			"msp", cfg.Fabric.MSPID,
		)
		fc, err := fabric.New(fabric.Config{
			PeerEndpoint: cfg.Fabric.PeerEndpoint,
			GatewayPeer:  cfg.Fabric.GatewayPeer,
			Channel:      cfg.Fabric.Channel,
			Chaincode:    cfg.Fabric.Chaincode,
			MSPID:        cfg.Fabric.MSPID,
			CertPath:     cfg.Fabric.CertPath,
			KeyDir:       cfg.Fabric.KeyDir,
			TLSCertPath:  cfg.Fabric.TLSCertPath,
		})
		if err != nil {
			slog.Error("Fabric connection failed, falling back to in-memory ledger", "error", err)
		} else {
			fabricClient = fc
			pool, hasPool := dbPool.(*pgxpool.Pool)

			// The adapter serves reads from the Postgres mirror when a pool is
			// present (fast), with Fabric as the authoritative backstop.
			var readPool *pgxpool.Pool
			if hasPool {
				readPool = pool
			}
			routerOpts = append(routerOpts, api.WithFabricLedger(fabric.NewLedgerAdapter(fc, readPool)))

			if height, err := fc.BlockHeight(syncCtx); err == nil {
				slog.Info("Fabric ledger connected", "block_height", height)
			}

			// Mirror committed chaincode events -> Postgres (off-chain read model).
			if hasPool {
				go fabric.NewSyncer(fc, pool, slog.Default()).Run(syncCtx)
				ledgerMode = "fabric+postgres"
			} else {
				slog.Warn("Fabric enabled without Postgres: off-chain ledger mirror disabled")
				ledgerMode = "fabric"
			}
		}
	} else {
		slog.Info("Fabric disabled (FABRIC_ENABLED != true), using in-memory ledger")
	}
	slog.Info("event ledger initialised", "ledger", ledgerMode)
	defer func() {
		if fabricClient != nil {
			_ = fabricClient.Close()
		}
	}()

	// ── JWT auth (spec §3.6) ─────────────────────────────────
	// Short-lived signed tokens replace opaque sessions. FINIX_JWT_SECRET is
	// mandatory — the server refuses to start rather than use an insecure
	// default.
	jwtManager, err := security.NewJWTManager()
	if err != nil {
		slog.Error("cannot start: JWT auth misconfigured", "error", err,
			"hint", "set FINIX_JWT_SECRET (>= 16 chars) to a strong secret")
		os.Exit(1)
	}
	// Persist revocations to Postgres when available (survives restart).
	if pool, ok := dbPool.(*pgxpool.Pool); ok {
		jwtManager.SetRevocationPersister(func(jti string, exp time.Time) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := pool.Exec(ctx,
				`INSERT INTO jwt_revocations (jti, expires_at) VALUES ($1,$2)
				 ON CONFLICT (jti) DO NOTHING`, jti, exp); err != nil {
				// Best-effort persistence; the in-memory set still denies the token
				// for the rest of its TTL. Log without the jti (no token material).
				slog.Warn("jwt revocation persist failed", "error", err)
			}
		})
		jwtManager.SetRevocationLookup(func(jti string) (bool, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var exists bool
			err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM jwt_revocations WHERE jti = $1)`, jti).Scan(&exists)
			return exists, err
		})
	}
	// Revocation-lookup failure policy (SF12): fail-closed denies a token when the
	// store is unreachable — the belt-and-suspenders banking posture. Default is
	// fail-open (availability first; the 15-min TTL bounds exposure of a token
	// revoked before a restart). Flip with FINIX_REVOCATION_FAIL_CLOSED=true.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("FINIX_REVOCATION_FAIL_CLOSED")), "true") {
		jwtManager.SetRevocationFailClosed(true)
		slog.Info("jwt revocation policy: fail-closed on lookup error")
	}
	routerOpts = append(routerOpts, api.WithJWTManager(jwtManager))
	slog.Info("JWT auth enabled", "ttl", security.TokenTTL.String())

	// ── AI RAG Integration ───────────────────────────────────
	// When AI_PROVIDER=remote, wire the Python FINIX RAG client.
	if cfg.AI.Provider == "remote" {
		if cfg.AI.RagBaseURL != "" && cfg.AI.InternalToken != "" {
			ragClient := ai.NewRagClient(cfg.AI)
			routerOpts = append(routerOpts, api.WithRagClient(ragClient))
			slog.Info("AI RAG client enabled", "upstream", cfg.AI.RagBaseURL)
		} else {
			slog.Warn("AI_PROVIDER=remote but AIML_UPSTREAM_URL or FINIX_INTERNAL_TOKEN not set, falling back to local")
		}
	} else {
		slog.Info("AI provider: local (in-process Groq chatbot)")
	}

	// ── Integration providers (bank / KYC / SIM) ─────────────
	// Selected by env; each defaults to the offline mock. A real provider is a
	// config-only flip (its stub returns a clear "not implemented" error rather
	// than faking success, proving the seam is wired).
	routerOpts = append(routerOpts, buildProviderOptions(cfg)...)

	// ── Redis outbox publisher ───────────────────────────────
	// Wire the Redis client and register it as the outbox publisher BEFORE
	// building the router, so the single Service.startOutboxWorker publishes
	// events to Redis Streams. Previously these options were appended to
	// routerOpts AFTER NewRouter had already consumed the slice, so they were
	// silently dropped and nothing published. There is now exactly one outbox
	// worker (in the service); the router's duplicate poller — which leaked on
	// context.Background().Done() — has been removed.
	var redisClient *redis.Client
	if cfg.Redis.URL != "" {
		opt, err := redis.ParseURL(cfg.Redis.URL)
		if err != nil {
			slog.Error("failed to parse Redis URL", "error", err)
		} else {
			if cfg.Redis.Password != "" {
				opt.Password = cfg.Redis.Password
			}
			opt.DB = cfg.Redis.DB
			redisClient = redis.NewClient(opt)
			// Test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := redisClient.Ping(ctx).Err(); err != nil {
				slog.Error("Redis connection failed", "error", err)
				redisClient = nil
			} else {
				slog.Info("Redis connected", "url", cfg.Redis.URL)
			}
			cancel()
		}
	}
	if redisClient != nil {
		routerOpts = append(routerOpts,
			api.WithRedisClient(redisClient),
			api.WithOutboxPublisher(func(ev *platform.OutboxEvent) error {
				return redisClient.XAdd(context.Background(), &redis.XAddArgs{
					Stream: ev.Topic,
					Values: map[string]interface{}{"payload": ev.Payload},
				}).Err()
			}),
		)
		slog.Info("outbox publisher wired to Redis Streams")
	}

	handler := api.NewRouter(dbPool, routerOpts...)

		server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.ReadTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	// Shutdown context: SIGINT/SIGTERM triggers graceful HTTP shutdown below.
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("server starting", "addr", cfg.Server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdownCtx.Done()

	slog.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	if pool, ok := dbPool.(interface{ Close() }); ok {
		pool.Close()
		slog.Info("database pool closed")
	}

	if redisClient != nil {
		redisClient.Close()
		slog.Info("Redis connection closed")
	}

	slog.Info("server stopped")
}

func setupLogging(cfg *config.Config) {
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// buildProviderOptions selects the bank / KYC / SIM integration providers from
// config and returns them as router options. Each defaults to the offline mock;
// a real provider name (razorpay / nsdl / telecom) selects the env-driven stub.
// An unknown value falls back to the mock with a warning.
func buildProviderOptions(cfg *config.Config) []api.RouterOption {
	var opts []api.RouterOption

	switch cfg.Providers.Bank {
	case "", "mock":
		opts = append(opts, api.WithBankAdapter(bank.NewMockBank()))
	case "razorpay":
		opts = append(opts, api.WithBankAdapter(bank.NewRazorpay(bank.RazorpayConfigFromEnv())))
		slog.Info("bank provider: razorpay (real-ready stub)")
	default:
		slog.Warn("unknown BANK_PROVIDER, defaulting to mock", "value", cfg.Providers.Bank)
		opts = append(opts, api.WithBankAdapter(bank.NewMockBank()))
	}

	switch cfg.Providers.KYC {
	case "", "mock":
		opts = append(opts, api.WithKYCProvider(kyc.NewMockKYC()))
	case "nsdl":
		opts = append(opts, api.WithKYCProvider(kyc.NewNSDL(kyc.NSDLConfigFromEnv())))
		slog.Info("kyc provider: nsdl (real-ready stub)")
	default:
		slog.Warn("unknown KYC_PROVIDER, defaulting to mock", "value", cfg.Providers.KYC)
		opts = append(opts, api.WithKYCProvider(kyc.NewMockKYC()))
	}

	switch cfg.Providers.SIM {
	case "", "mock":
		opts = append(opts, api.WithSIMVerifier(sim.NewMockSIM()))
	case "telecom":
		opts = append(opts, api.WithSIMVerifier(sim.NewTelecom(sim.TelecomConfigFromEnv())))
		slog.Info("sim provider: telecom (real-ready stub)")
	default:
		slog.Warn("unknown SIM_PROVIDER, defaulting to mock", "value", cfg.Providers.SIM)
		opts = append(opts, api.WithSIMVerifier(sim.NewMockSIM()))
	}

	slog.Info("integration providers",
		"bank", firstNonEmptyStr(cfg.Providers.Bank, "mock"),
		"kyc", firstNonEmptyStr(cfg.Providers.KYC, "mock"),
		"sim", firstNonEmptyStr(cfg.Providers.SIM, "mock"),
		"ai", firstNonEmptyStr(cfg.AI.Provider, "local"),
	)
	return opts
}

func firstNonEmptyStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}