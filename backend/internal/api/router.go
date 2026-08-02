package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	apimw "FINIX/backend/internal/api/middleware"
	"FINIX/backend/internal/config"
	"FINIX/backend/internal/domain/blockchain"
	"FINIX/backend/internal/domain/platform"
	"FINIX/backend/internal/domain/security"
	"FINIX/backend/internal/infra/ai"
)

type API struct {
	svc         *platform.Service
	pqc         *security.PQCManager
	dilithium   *security.DilithiumManager
	flow        *flowTracker
	aiml        *aimlProxy    // staged proxy of LLM/AIML routes to the Python AI ecosystem
	ragClient   *ai.RagClient // Python FINIX RAG service client
	redisClient *redis.Client // Redis client for outbox publishing
}

// forwardableHeaders is the allow-list of headers proxied to the expanded-endpoint
// service (L-6). Everything else — auth/session tokens, cookies, and spoofable
// proxy headers like X-Forwarded-For — is dropped.
var forwardableHeaders = map[string]struct{}{
	"Content-Type":             {},
	"Idempotency-Key":          {},
	"X-Device-Fingerprint":     {},
	"X-Device-Trusted":         {},
	"X-Biometric-Challenge-Id": {},
	"X-Biometric-Challenge":    {},
}

type healthResponse struct {
	Status     string            `json:"status"`
	Service    string            `json:"service"`
	Version    string            `json:"version"`
	Timestamp  time.Time         `json:"timestamp"`
	Components map[string]string `json:"components,omitempty"`
}

type versionResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
}

// RouterOption customises the router at construction time.
type RouterOption func(*routerOptions)

type routerOptions struct {
	fabricLedger    blockchain.FabricBackend
	bank            platform.BankAdapter
	kyc             security.KYCProvider
	sim             security.SIMVerifier
	jwt             *security.JWTManager
	ragClient       *ai.RagClient
	redisClient     *redis.Client
	outboxPublisher func(*platform.OutboxEvent) error
}

// WithJWTManager makes login issue signed JWTs (spec §3.6) instead of opaque
// tokens, and enables structured JWT verification in the Auth middleware.
func WithJWTManager(m *security.JWTManager) RouterOption {
	return func(o *routerOptions) { o.jwt = m }
}

// WithFabricLedger makes Hyperledger Fabric the system of record for the event
// ledger instead of the in-memory chain.
func WithFabricLedger(backend blockchain.FabricBackend) RouterOption {
	return func(o *routerOptions) { o.fabricLedger = backend }
}

// WithBankAdapter / WithKYCProvider / WithSIMVerifier inject the integration
// providers chosen at the composition root (mock by default, real by env).
func WithBankAdapter(b platform.BankAdapter) RouterOption {
	return func(o *routerOptions) { o.bank = b }
}
func WithKYCProvider(k security.KYCProvider) RouterOption {
	return func(o *routerOptions) { o.kyc = k }
}
func WithSIMVerifier(s security.SIMVerifier) RouterOption {
	return func(o *routerOptions) { o.sim = s }
}

// WithRagClient injects the RAG client for AI ecosystem integration.
// When set, the Python FINIX RAG service handles chat, SMS scanning, and risk scoring.
func WithRagClient(c *ai.RagClient) RouterOption {
	return func(o *routerOptions) { o.ragClient = c }
}

// WithRedisClient injects the Redis client for outbox event publishing.
func WithRedisClient(c *redis.Client) RouterOption {
	return func(o *routerOptions) { o.redisClient = c }
}

// WithOutboxPublisher injects the outbox publisher function for publishing events to Redis Streams.
func WithOutboxPublisher(publisher func(*platform.OutboxEvent) error) RouterOption {
	return func(o *routerOptions) { o.outboxPublisher = publisher }
}

// structured-verify contract. Returns nil when no JWT manager is injected, so
// the middleware falls back to opaque-token authentication (unit tests).
func jwtVerifier(svc *platform.Service) apimw.JWTVerify {
	jm := svc.JWTManager()
	if jm == nil {
		return nil
	}
	return func(token, deviceFP string) (*apimw.TokenClaims, string, bool) {
		claims, err := jm.VerifyToken(token, deviceFP)
		if err == nil {
			return &apimw.TokenClaims{
				UserID:   claims.Sub,
				Role:     claims.Role,
				DeviceFP: claims.DeviceFP,
			}, "", true
		}
		switch {
		case errors.Is(err, security.ErrTokenExpired):
			return nil, "token_expired", false
		case errors.Is(err, security.ErrDeviceMismatch):
			return nil, "device_mismatch", false
		case errors.Is(err, security.ErrTokenRevoked):
			return nil, "token_revoked", false
		default:
			// Not a JWT we issued (e.g. an OAuth/legacy opaque token) — let the
			// middleware fall through to the legacy authenticator.
			return nil, "token_invalid", false
		}
	}
}

// isDevEnv reports whether the server is running in a non-production environment.
// Demo/diagnostic routes are only registered when this is true. Defaults to false
// (production-safe) unless FINIX_ENV is explicitly dev/development/local/test.
func isDevEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_ENV"))) {
	case "dev", "development", "local", "test":
		return true
	default:
		return false
	}
}

// NewRouter builds the HTTP API. Variadic options keep the original
// NewRouter(dbPool) call sites working unchanged.
func NewRouter(dbPool any, opts ...RouterOption) http.Handler {
	var options routerOptions
	for _, opt := range opts {
		opt(&options)
	}

	// Load CORS allowed origins from environment. Kept as a local (per-router)
	// slice — not package state — so building a second router never accumulates
	// or shares origins across instances.
	var allowedOrigins []string
	originsRaw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if originsRaw != "" {
		for _, o := range strings.Split(originsRaw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}

	// Cap request bodies (default 1 MiB) so an oversized/streaming payload can't
	// exhaust memory. Override with MAX_REQUEST_BYTES.
	maxRequestBytes := int64(1 << 20)
	if v := strings.TrimSpace(os.Getenv("MAX_REQUEST_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxRequestBytes = n
		}
	}

	var pqcManager *security.PQCManager
	pqcManager, err := security.NewPQCManager()
	if err != nil {
		slog.Error("failed to initialize PQC manager, PQC endpoints will be unavailable", "error", err)
		pqcManager = nil
	}

	var dilithiumMgr *security.DilithiumManager
	dilithiumMgr, err = security.NewDilithiumManager()
	if err != nil {
		slog.Error("failed to initialize Dilithium manager, signature endpoints will be unavailable", "error", err)
		dilithiumMgr = nil
	}

	// Versioned model parameters (spec "Srishti Math"): load once here (env
	// overrides applied) and thread into the service so every threshold/weight is
	// auditable config, and the Version propagates into snapshots + simulation
	// metadata. Falls back to the validated default on any inconsistency.
	modelParams := config.LoadModelParams()
	if err := modelParams.Validate(); err != nil {
		slog.Warn("model params invalid, using defaults", "error", err)
		modelParams = config.Default()
	} else {
		slog.Info("model params loaded", "version", modelParams.Version)
	}

	svc := platform.NewService()
	if dbPool != nil {
		if pool, ok := dbPool.(*pgxpool.Pool); ok {
			svc = platform.NewServiceWithDB(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := svc.HydrateFromDB(ctx); err != nil {
				slog.Warn("DB hydration had errors, continuing with partial data", "error", err)
			}
			cancel()
		}
	}
	svc.UseModelParams(modelParams)

	// Attach Hyperledger Fabric as the ledger's system of record before any
	// seeding, so seeded events are written on-chain too.
	if options.fabricLedger != nil {
		svc.UseFabricLedger(options.fabricLedger)
		slog.Info("event ledger backed by Hyperledger Fabric")
	}

	// Inject the integration providers (bank / KYC / SIM) before seeding.
	if options.bank != nil {
		svc.UseBankAdapter(options.bank)
	}
	if options.kyc != nil {
		svc.UseKYCProvider(options.kyc)
	}
	if options.sim != nil {
		svc.UseSIMVerifier(options.sim)
	}
	if options.ragClient != nil {
		svc.UseRagClient(options.ragClient)
		slog.Info("AI ecosystem: FINIX RAG client attached")
	}
	if options.jwt != nil {
		svc.UseJWTManager(options.jwt)
		slog.Info("auth tokens: signed JWT (HS256, 15-min expiry)")
	}
	// Wire the outbox publisher if provided
	if options.outboxPublisher != nil {
		svc.SetOutboxPublisher(options.outboxPublisher)
		slog.Info("outbox publisher attached for Redis Streams publishing")
	}

	if svc.NeedsSeeding() {
		slog.Info("no users found, seeding demo users...")
		if err := svc.SeedDemoUsers(); err != nil {
			slog.Warn("demo user seeding had errors", "error", err)
		}
	}

	tracker := newFlowTracker(300)
	api := &API{
		svc:         svc,
		pqc:         pqcManager,
		dilithium:   dilithiumMgr,
		flow:        tracker,
		aiml:        newAIMLProxy(),
		ragClient:   options.ragClient,
		redisClient: options.redisClient,
	}
	if api.aiml.enabled {
		slog.Info("AIML proxy ENABLED: LLM/AIML routes forward to the Python AI ecosystem", "upstream", api.aiml.upstream)
	} else {
		slog.Info("AIML proxy disabled: LLM/AIML routes served in-process (set AIML_PROXY_ENABLED=true to migrate)")
	}
	if api.ragClient != nil {
		slog.Info("FINIX RAG client attached to API", "baseURL", api.ragClient.BaseURL())
	}
	if api.redisClient != nil {
		// Outbox publishing is handled by the single Service.startOutboxWorker via
		// the wired outboxPublisher (see WithOutboxPublisher / cmd/server/main.go).
		// We deliberately do NOT start a second poller here: the previous one both
		// leaked (it selected on context.Background().Done(), which never fires) and
		// competed with the service worker for the same outbox.
		slog.Info("Redis client attached; outbox is published by the service worker")
	}
	if dilithiumMgr != nil {
		svc.SetDilithiumManager(dilithiumMgr)
	}
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)
	r.Use(corsMiddleware(allowedOrigins))
	r.Use(securityHeaders)
	r.Use(csrfDefense)
	r.Use(maxBodyBytes(maxRequestBytes))
	r.Use(apimw.RateLimit(240, time.Minute))
	r.Use(api.flow.Middleware)

	// Certificate pinning validation (spec §3.6)
	// Certificate pinning (spec §3.6) — enforced on EVERY route via the global
	// chain. Pins come from CERT_PINS (comma-separated SHA-256 fingerprints); a
	// request presenting an X-Cert-Fingerprint outside the set is rejected.
	certPinMgr := apimw.NewCertificatePinningManagerFromEnv()
	if certPinMgr.Enabled() {
		slog.Info("certificate pinning enabled", "pins", certPinMgr.PinCount())
	} else {
		slog.Warn("certificate pinning disabled (CERT_PINS not set) — client-side pinning still applies")
	}
	r.Use(apimw.PinVerificationMiddleware(certPinMgr))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		components := map[string]string{
			"postgres":  "not_configured",
			"pqc":       "available",
			"dilithium": "available",
		}
		if api.pqc == nil {
			components["pqc"] = "unavailable"
		}
		if api.dilithium == nil {
			components["dilithium"] = "unavailable"
		}
		if api.svc != nil && api.svc.IsDBConnected() {
			components["postgres"] = "connected"
		}
		writeJSON(w, http.StatusOK, healthResponse{
			Status:     "ok",
			Service:    "FINIX-backend",
			Version:    "0.2.0",
			Timestamp:  time.Now().UTC(),
			Components: components,
		})
	})

	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		components := map[string]string{
			"postgres":  "not_configured",
			"pqc":       "available",
			"dilithium": "available",
		}
		if api.pqc == nil {
			components["pqc"] = "unavailable"
		}
		if api.dilithium == nil {
			components["dilithium"] = "unavailable"
		}
		if api.svc != nil && api.svc.IsDBConnected() {
			components["postgres"] = "connected"
		}
		writeJSON(w, http.StatusOK, healthResponse{
			Status:     "ready",
			Service:    "FINIX-backend",
			Version:    "0.2.0",
			Timestamp:  time.Now().UTC(),
			Components: components,
		})
	})

	// L-1/L-2 fix: openapi spec, deep health and metrics expose internal state and
	// the full API surface. Only register them in non-production environments.
	if isDevEnv() {
		r.Get("/v1/system/openapi.json", api.openAPISpec)
		r.Get("/v1/system/metrics", api.metrics)
		r.Get("/v1/system/health-deep", api.healthDeep)
	}

	// Prometheus metrics endpoint (always available for production monitoring)
	r.Get("/metrics", api.metrics)

	// Dummy Banking API (simulates real bank for demo).
	// H-11 fix: these expose UPI/IFSC/balance lookups and must NOT be reachable in
	// production. Only register them when FINIX_ENV is a non-production/dev value.
	if isDevEnv() {
		r.Get("/v1/dummy-bank/verify-upi", api.dummyBankVerifyUPI)
		r.Get("/v1/dummy-bank/verify-ifsc", api.dummyBankVerifyIFSC)
		r.Get("/v1/dummy-bank/balance", api.dummyBankBalance)
	}

	r.Get("/v1/system/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, versionResponse{Service: "FINIX-backend", Version: "0.2.0"})
	})
	if isDevEnv() {
		r.Get("/v1/system/flow-dashboard", api.flowDashboardData)
		r.Get("/ops/flow-dashboard", api.flowDashboardPage)
	}
	r.Get("/v1/settings/consent/privacy_policy/status", api.expandedEndpoint("consent_pp_status", false, false))
	r.Post("/v1/settings/consent/privacy_policy", api.setPublicPrivacyConsent)

	// Public onboarding/auth routes.
	// Dedicated brute-force limiter shared across all credential-verification
	// endpoints (per client IP), far stricter than the global 240/min. An
	// attacker cannot spread guesses across endpoints to evade it.
	authLimiter := apimw.RateLimit(20, time.Minute)
	// Registration/onboarding is write-heavy but should not be hammered either.
	registerLimiter := apimw.RateLimit(10, time.Minute)

	r.With(authLimiter).Post("/v1/auth/sim-bind", api.expandedEndpoint("auth_sim_bind", false, true))
	r.With(registerLimiter).Post("/v1/auth/register", api.register)
	r.With(authLimiter).Post("/v1/auth/ekyc/verify", api.verifyEKYC)
	r.With(registerLimiter).Post("/v1/auth/biometric/register", api.registerBiometric)
	r.With(authLimiter).Post("/v1/auth/login/challenge", api.loginChallenge)
	r.With(authLimiter).Post("/v1/auth/login/pin", api.pinLogin)
	r.With(authLimiter).Post("/v1/auth/login/pin/set", api.pinSet)
	r.With(authLimiter).Post("/v1/auth/login/verify", api.loginVerify)
	// Public: an expired JWT can't pass Auth, so refresh must be reachable
	// without it (still gated by device fingerprint + the grace window).
	r.With(authLimiter).Post("/v1/auth/refresh", api.refreshToken)
	r.With(authLimiter).Post("/v1/oauth/token", api.oauthToken)
	r.With(authLimiter).Post("/v1/oauth/authorize", api.oauthAuthorize)

	r.Get("/v1/auth/pqc/init", api.pqcInit)
	r.Post("/v1/auth/pqc/encapsulate", api.pqcEncapsulate)
	r.Get("/v1/auth/pqc/key-info", api.pqcKeyInfo)
	r.Get("/v1/auth/dilithium/public-key", api.dilithiumPublicKey)

	// Internal service-to-service routes. Mounted under /v1/internal so the
	// mount does not shadow the user-facing /v1/blockchain/* routes that live
	// inside the authenticated /v1 group (chi routes to the most specific
	// mount, which previously swallowed /v1/blockchain/news-whitelist).
	r.Route("/v1/internal/blockchain", func(internal chi.Router) {
		internal.Use(apimw.RequireScopedToken("X-Internal-Token", tokenOrDefault("FINIX_INTERNAL_TOKEN")))
		internal.Post("/consent-record", api.expandedEndpoint("blockchain_consent_record", false, true))
		internal.Post("/insurance-claim", api.expandedEndpoint("blockchain_insurance_claim", false, true))
		internal.Post("/loan-repayment", api.expandedEndpoint("blockchain_loan_repayment", false, true))
		internal.Post("/investment-trade", api.expandedEndpoint("blockchain_investment_trade", false, true))
		internal.Get("/cooling-off/{txID}", api.expandedEndpoint("blockchain_cooling_off", false, false, "txID"))
	})

	// H-5 fix: the static X-Admin-Token establishes no user identity. Layer a
	// per-user JWT + server-side RBAC on top, so an admin action requires BOTH the
	// service token AND an authenticated principal holding a privileged role. The
	// token alone (e.g. discovered in a log) no longer grants access.
	r.Route("/v1/compliance", func(admin chi.Router) {
		admin.Use(apimw.RequireScopedToken("X-Admin-Token", tokenOrDefault("FINIX_ADMIN_TOKEN")))
		admin.Use(apimw.Auth(jwtVerifier(api.svc), api.svc.AuthenticateToken, api.svc.GetRole))
		admin.Use(apimw.RequireRole(apimw.RoleComplianceOfficer, apimw.RoleAdmin))
		admin.Post("/breach-notify", api.expandedEndpoint("compliance_breach_notify", false, true))
	})

	// Internal risk validation endpoint - called by Python RAG validation worker
	// Requires FINIX_INTERNAL_TOKEN for service-to-service auth
	r.Route("/v1/internal/risk-validation", func(internal chi.Router) {
		internal.Use(apimw.RequireScopedToken("X-Internal-Token", tokenOrDefault("FINIX_INTERNAL_TOKEN")))
		internal.Post("/", api.validateRisk)
	})

	// Admin-only AIML audit. Mounted under /v1/admin so it does not shadow
	// the user-facing /v1/aiml/* routes (behaviour/predict,
	// investments/recommend, synthetic/generate, bank/readiness).
	r.Route("/v1/admin/aiml", func(admin chi.Router) {
		admin.Use(apimw.RequireScopedToken("X-Admin-Token", tokenOrDefault("FINIX_ADMIN_TOKEN")))
		admin.Use(apimw.Auth(jwtVerifier(api.svc), api.svc.AuthenticateToken, api.svc.GetRole))
		admin.Use(apimw.RequireRole(apimw.RoleAdmin, apimw.RoleAuditor))
		admin.Get("/bias-audit", api.expandedEndpoint("aiml_bias_audit", false, false))
	})

	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(apimw.Auth(jwtVerifier(api.svc), api.svc.AuthenticateToken, api.svc.GetRole))
		// Post-quantum session enforcement (defence-in-depth on top of TLS).
		// When FINIX_REQUIRE_PQC is enabled and the Kyber-1024 KEM manager is
		// available, every authenticated request must carry a valid
		// X-PQC-Session obtained via the /v1/auth/pqc/* handshake.
		if api.pqc != nil && pqcEnforced() {
			v1.Use(apimw.RequirePQCSession(api.pqc.ValidatePQCAuth))
			slog.Info("PQC session enforcement ENABLED for /v1 (Kyber-1024)")
		}
		v1.Use(apimw.RequireIdempotency("/v1/transactions", "/v1/security/emergency-freeze", "/v1/security/unfreeze"))

		v1.Get("/dashboard", api.dashboard)
		v1.Get("/auth/profile", api.authProfile)
		v1.Patch("/auth/profile", api.updateAuthProfile)
		v1.Post("/auth/stepup/challenge", api.stepupChallenge)
		v1.Post("/auth/otp/generate", api.otpGenerate)
		v1.Get("/kyc/profile", api.kycProfile)
		v1.Patch("/kyc/profile", api.upsertKYCProfile)

		v1.Get("/accounts", api.listAccounts)
		v1.Post("/accounts/link", api.linkAccount)
		v1.Patch("/accounts/{accountID}", api.updateAccount)

		v1.Post("/beneficiaries", api.createBeneficiary)
		v1.Get("/beneficiaries", api.listBeneficiaries)
		v1.Post("/beneficiaries/{beneficiaryID}/approve", api.approveBeneficiary)

		v1.Get("/aggregator/status", api.aggregatorStatus)
		v1.Post("/aggregator/sync", api.syncAggregator)

		v1.Get("/aiml/behaviour/predict", api.predictBehaviour)
		v1.Get("/aiml/investments/recommend", api.recommendInvestments)
		v1.Post("/aiml/synthetic/generate", api.generateSyntheticDataset)
		v1.Get("/aiml/bank/readiness", api.bankAPIReadiness)

		v1.Post("/bank/connect", api.connectBankAPI)
		v1.Get("/bank/connection", api.bankConnection)
		v1.Post("/bank/events/ingest", api.ingestBankEvent)
		v1.Get("/bank/events/detections", api.realtimeDetections)
		v1.Post("/bank/events/synthetic-stream", api.syntheticBankEventStream)

		v1.Post("/payments/initiate", api.initiatePayment)
		v1.Get("/payments", api.listPayments)
		v1.Get("/payments/{paymentID}", api.getPayment)
		v1.Post("/payments/{paymentID}/verify-receipt", api.verifyPaymentReceipt)

		v1.With(apimw.EnforceCoolingOff("", api.svc.CoolingOffState)).Post("/transactions/initiate", api.initiateTransaction)
		// H-7 fix: override must ALSO be gated by cooling-off. Previously only
		// /initiate was gated, so a user could initiate → get blocked → immediately
		// call /override to bypass the cooling-off window entirely.
		v1.With(apimw.EnforceCoolingOff("", api.svc.CoolingOffState)).Post("/transactions/override", api.overrideTransaction)
		v1.Get("/transactions/history", api.transactionHistory)

		v1.Get("/health-score", api.healthScore)

		v1.Post("/goals", api.createGoal)
		v1.Get("/goals", api.listGoals)
		v1.Get("/goals/{goalID}", api.goalByID)
		v1.Post("/goals/{goalID}/contribute", api.contributeGoal)
		v1.Post("/goals/{goalID}/close", api.closeGoal)
		v1.Post("/goals/{goalID}/dissolve", api.dissolveGoal)
		v1.Post("/goals/{goalID}/pause", api.pauseGoal)
		v1.Post("/goals/{goalID}/resume", api.resumeGoal)
		v1.Post("/goals/{goalID}/nudge", api.goalNudge)

		v1.Get("/portfolio/summary", api.portfolioSummary)
		v1.Get("/portfolio/investments", api.portfolioInvestments)
		v1.Get("/portfolio/insurance", api.portfolioInsurance)
		v1.Get("/portfolio/loans", api.portfolioLoans)
		v1.Get("/portfolio/audit-logs", api.auditLogs)
		v1.Get("/portfolio/net-worth", api.netWorthSnapshot)
		v1.Post("/portfolio/assets", api.addNetWorthAsset)
		v1.Delete("/portfolio/assets/{assetID}", api.removeNetWorthAsset)

		v1.Post("/simulations/run", api.runSimulation)
		v1.Get("/simulations/scenarios", api.availableScenarios)

		v1.Get("/insights/feed", api.insightsFeed)
		v1.Get("/insights/market", api.insightsMarket)
		v1.Get("/insights/persona", api.insightsPersona)

		v1.Get("/market/news", api.marketNews)
		v1.Get("/market/portfolio-impact", api.portfolioImpact)
		v1.Get("/market/narration", api.marketNarration)
		v1.Get("/market/trends", api.marketTrends)

		v1.Get("/security/health", api.securityHealth)
		v1.Post("/security/sms/scan", api.scanSMS)
		v1.Post("/security/emergency-freeze", api.emergencyFreeze)
		v1.With(apimw.RequireBiometricChallenge(60*time.Second, api.svc.VerifyBiometricChallenge)).Post("/security/unfreeze", api.unfreeze)
		v1.Post("/security/report-fraud", api.reportFraud)
		v1.Get("/security/emergency-contacts", api.emergencyContacts)
		v1.Post("/security/emergency-contacts", api.addEmergencyContact)
		v1.Delete("/security/emergency-contacts/{contactID}", api.deleteEmergencyContact)
		v1.Get("/security/tips", api.securityTips)

		v1.Get("/tax/dashboard", api.taxDashboard)
		v1.Get("/tax/regime-compare", api.taxRegimeCompare)
		v1.Get("/tax/deductions", api.taxDeductions)
		v1.Get("/tax/capital-gains", api.taxCapitalGains)

		// AIML migration (AI_ECOSYSTEM_ARCHITECTURE.md): the chatbot is an LLM
		// service that belongs to the Python AI ecosystem. Routed through the proxy
		// seam — forwards to the upstream when AIML_PROXY_ENABLED=true, otherwise
		// served in-process. Other /v1/aiml/* + /chatbot/* routes follow the same
		// one-line `api.aiml.route(<python-path>, <local-handler>)` pattern once the
		// matching Python endpoints exist.
		v1.Post("/chatbot/query", api.aiml.route("/query", api.chat))

		v1.Get("/settings/profile", api.profile)
		v1.Post("/settings/consent/{consentType}", api.setConsent)
		v1.Post("/settings/nudge-preference", api.setNudgePreference)
		v1.Get("/settings/notifications", api.notificationSettings)
		v1.Post("/settings/feedback", api.submitFeedback)
		v1.Get("/settings/feature-flags", api.featureFlags)
		v1.Get("/settings/help", api.helpArticles)

		v1.Get("/notifications", api.notificationCentre)
		v1.Post("/notifications/{notificationID}/dismiss", api.dismissNotification)
		v1.Get("/notifications/stream", api.sseStream)

		v1.Get("/audit/logs", api.auditLogs)
		v1.Get("/audit/integrity", api.auditIntegrity)
		v1.Get("/audit/ledger", api.ledgerEntries)

		api.registerExpandedAuthedRoutes(v1)

		v1.Get("/features", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "active",
				"coverage": []string{
					"onboarding", "dashboard", "transaction-risk", "goals", "health-score",
					"portfolio", "net-worth", "market-news", "simulation", "insights", "security-corner",
					"tax", "chatbot", "settings", "audit-log", "blockchain-integrity",
					"beneficiaries", "account-aggregator", "notifications",
					"aiml-security", "aiml-behaviour", "aiml-investment-recommendations",
					"synthetic-dataset-simulation", "bank-api-connector", "realtime-detection",
				},
			})
		})
	})
	return r
}

func (api *API) netWorthSnapshot(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.NetWorthSnapshot(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) addNetWorthAsset(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.NetWorthAsset
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.AddNetWorthAsset(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) removeNetWorthAsset(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	assetID := chi.URLParam(r, "assetID")
	if err := api.svc.RemoveNetWorthAsset(uid, assetID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (api *API) availableScenarios(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.AvailableScenarios())
}

func (api *API) marketNews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.MarketNews())
}

func (api *API) portfolioImpact(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.PortfolioImpact(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) marketNarration(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.MarketNarration())
}

func (api *API) marketTrends(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"trends": []map[string]any{
			{"indicator": "SENSEX", "direction": "up", "strength": "moderate", "period": "7d"},
			{"indicator": "GOLD", "direction": "down", "strength": "weak", "period": "7d"},
			{"indicator": "REPO_RATE", "direction": "stable", "strength": "strong", "period": "60d"},
		},
	})
}

func (api *API) emergencyContacts(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.svc.EmergencyContacts(uid))
}

func (api *API) addEmergencyContact(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.EmergencyContact
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.AddEmergencyContact(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) deleteEmergencyContact(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	contactID := chi.URLParam(r, "contactID")
	if err := api.svc.DeleteEmergencyContact(uid, contactID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (api *API) securityTips(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.SecurityTips())
}

func (api *API) dissolveGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	var req platform.GoalDissolveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.DissolveGoal(uid, goalID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) pauseGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	res, err := api.svc.PauseGoal(uid, goalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) resumeGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	res, err := api.svc.ResumeGoal(uid, goalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) goalNudge(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	var req platform.GoalNudgeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.GoalNudge(uid, goalID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) featureFlags(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.svc.FeatureFlags(uid))
}

func (api *API) helpArticles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.HelpArticles())
}

func (api *API) notificationCentre(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	res, err := api.svc.NotificationCentre(uid, page, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) dismissNotification(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	notificationID := chi.URLParam(r, "notificationID")
	if err := api.svc.DismissNotification(uid, notificationID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (api *API) sseStream(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	// L-5: enforce the per-user connection cap before committing to the stream.
	ch := api.svc.SubscribeSSE(uid)
	if ch == nil {
		writeError(w, http.StatusTooManyRequests, "too many concurrent event streams")
		return
	}
	defer api.svc.UnsubscribeSSE(uid, ch)

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event, string(data))
			rc.Flush()
		}
	}
}

func (api *API) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "FINIX SecureWealth Twin API",
			"version":     "0.2.0",
			"description": "Backend API for FINIX — India's first PQC-secured, AI-powered wealth management platform.",
		},
		"servers": []map[string]any{{"url": "/v1"}},
		"paths": map[string]any{
			"/dashboard":                      map[string]any{"get": map[string]any{"summary": "Home Dashboard", "tags": []string{"Dashboard"}}},
			"/accounts":                       map[string]any{"get": map[string]any{"summary": "List Accounts"}, "post": map[string]any{"summary": "Link Account"}},
			"/accounts/{accountID}":           map[string]any{"patch": map[string]any{"summary": "Update Account"}},
			"/transactions/initiate":          map[string]any{"post": map[string]any{"summary": "Initiate Transaction", "tags": []string{"Transactions"}}},
			"/transactions/override":          map[string]any{"post": map[string]any{"summary": "Override Transaction"}},
			"/transactions/history":           map[string]any{"get": map[string]any{"summary": "Transaction History"}},
			"/payments/initiate":              map[string]any{"post": map[string]any{"summary": "Initiate Payment", "tags": []string{"Payments"}}},
			"/payments":                       map[string]any{"get": map[string]any{"summary": "List Payments"}},
			"/goals":                          map[string]any{"get": map[string]any{"summary": "List Goals"}, "post": map[string]any{"summary": "Create Goal"}},
			"/goals/{goalID}":                 map[string]any{"get": map[string]any{"summary": "Get Goal"}},
			"/goals/{goalID}/dissolve":        map[string]any{"post": map[string]any{"summary": "Dissolve Goal"}},
			"/goals/{goalID}/nudge":           map[string]any{"post": map[string]any{"summary": "Goal Nudge"}},
			"/goals/{goalID}/pause":           map[string]any{"post": map[string]any{"summary": "Pause Goal"}},
			"/goals/{goalID}/resume":          map[string]any{"post": map[string]any{"summary": "Resume Goal"}},
			"/portfolio/net-worth":            map[string]any{"get": map[string]any{"summary": "Net Worth Snapshot", "tags": []string{"Portfolio"}}},
			"/portfolio/assets":               map[string]any{"post": map[string]any{"summary": "Add Asset"}},
			"/portfolio/investments":          map[string]any{"get": map[string]any{"summary": "List Investments"}},
			"/portfolio/insurance":            map[string]any{"get": map[string]any{"summary": "List Insurance"}},
			"/portfolio/loans":                map[string]any{"get": map[string]any{"summary": "List Loans"}},
			"/simulations/run":                map[string]any{"post": map[string]any{"summary": "Run Simulation", "tags": []string{"Simulation"}}},
			"/simulations/scenarios":          map[string]any{"get": map[string]any{"summary": "Available Scenarios"}},
			"/insights/feed":                  map[string]any{"get": map[string]any{"summary": "Insights Feed", "tags": []string{"Insights"}}},
			"/insights/market":                map[string]any{"get": map[string]any{"summary": "Market Snapshot"}},
			"/insights/persona":               map[string]any{"get": map[string]any{"summary": "Financial Persona"}},
			"/market/news":                    map[string]any{"get": map[string]any{"summary": "Market News", "tags": []string{"Market"}}},
			"/market/portfolio-impact":        map[string]any{"get": map[string]any{"summary": "Portfolio Impact"}},
			"/market/narration":               map[string]any{"get": map[string]any{"summary": "Market Narration"}},
			"/market/trends":                  map[string]any{"get": map[string]any{"summary": "Market Trends"}},
			"/security/health":                map[string]any{"get": map[string]any{"summary": "Security Health", "tags": []string{"Security"}}},
			"/security/sms/scan":              map[string]any{"post": map[string]any{"summary": "Scan SMS"}},
			"/security/emergency-freeze":      map[string]any{"post": map[string]any{"summary": "Emergency Freeze"}},
			"/security/unfreeze":              map[string]any{"post": map[string]any{"summary": "Unfreeze"}},
			"/security/emergency-contacts":    map[string]any{"get": map[string]any{"summary": "Emergency Contacts"}, "post": map[string]any{"summary": "Add Contact"}},
			"/security/tips":                  map[string]any{"get": map[string]any{"summary": "Security Tips"}},
			"/tax/dashboard":                  map[string]any{"get": map[string]any{"summary": "Tax Dashboard", "tags": []string{"Tax"}}},
			"/tax/regime-compare":             map[string]any{"get": map[string]any{"summary": "Regime Comparison"}},
			"/tax/deductions":                 map[string]any{"get": map[string]any{"summary": "Deductions"}},
			"/tax/capital-gains":              map[string]any{"get": map[string]any{"summary": "Capital Gains"}},
			"/chatbot/query":                  map[string]any{"post": map[string]any{"summary": "Chatbot Query", "tags": []string{"AI"}}},
			"/settings/feature-flags":         map[string]any{"get": map[string]any{"summary": "Feature Flags", "tags": []string{"Settings"}}},
			"/settings/help":                  map[string]any{"get": map[string]any{"summary": "Help Articles"}},
			"/settings/consent/{consentType}": map[string]any{"post": map[string]any{"summary": "Set Consent"}},
			"/notifications":                  map[string]any{"get": map[string]any{"summary": "Notification Centre", "tags": []string{"Notifications"}}},
			"/notifications/stream":           map[string]any{"get": map[string]any{"summary": "SSE Event Stream"}},
			"/audit/logs":                     map[string]any{"get": map[string]any{"summary": "Audit Logs", "tags": []string{"Audit"}}},
			"/audit/integrity":                map[string]any{"get": map[string]any{"summary": "Audit Integrity"}},
		},
	}
	writeJSON(w, http.StatusOK, spec)
}

func (api *API) healthDeep(w http.ResponseWriter, r *http.Request) {
	probes := map[string]any{
		"service": "FINIX-backend",
		"version": "0.2.0",
		"uptime":  time.Now().UTC().Format(time.RFC3339),
	}
	dbStatus := "not_configured"
	if api.svc != nil && api.svc.IsDBConnected() {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := api.svc.DBPing(ctx); err != nil {
			dbStatus = "unreachable"
		} else {
			dbStatus = "connected"
		}
	}
	probes["postgres"] = dbStatus

	pqcAge := "available"
	if api.pqc == nil {
		pqcAge = "unavailable"
	} else {
		ki := api.pqc.GetKeyInfo()
		pqcAge = fmt.Sprintf("key_age=%.0fh", time.Since(ki.KeyRotatedAt).Hours())
	}
	probes["pqc"] = pqcAge

	dilithiumStatus := "available"
	if api.dilithium == nil {
		dilithiumStatus = "unavailable"
	} else {
		ki := api.dilithium.GetCurrentKeyInfo()
		dilithiumStatus = fmt.Sprintf("key_age=%.0fh", time.Since(ki.RotatedAt).Hours())
	}
	probes["dilithium"] = dilithiumStatus

	if api.svc != nil {
		stats := api.svc.SystemStats()
		for k, v := range stats {
			probes[k] = v
		}
	}
	writeJSON(w, http.StatusOK, probes)
}

func (api *API) metrics(w http.ResponseWriter, _ *http.Request) {
	if api.svc == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "service not initialized"})
		return
	}
	stats := api.svc.SystemStats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP finix_users_total Total registered users\n")
	fmt.Fprintf(w, "# TYPE finix_users_total gauge\n")
	fmt.Fprintf(w, "finix_users_total %d\n", intFromMap(stats, "user_count"))
	fmt.Fprintf(w, "# HELP finix_transactions_total Total transactions\n")
	fmt.Fprintf(w, "# TYPE finix_transactions_total gauge\n")
	fmt.Fprintf(w, "finix_transactions_total %d\n", intFromMap(stats, "transaction_count"))
	fmt.Fprintf(w, "# HELP finix_active_sessions Active user sessions\n")
	fmt.Fprintf(w, "# TYPE finix_active_sessions gauge\n")
	fmt.Fprintf(w, "finix_active_sessions %d\n", intFromMap(stats, "active_sessions"))
	fmt.Fprintf(w, "# HELP finix_fraud_graph_nodes Fraud graph node count\n")
	fmt.Fprintf(w, "# TYPE finix_fraud_graph_nodes gauge\n")
	fmt.Fprintf(w, "finix_fraud_graph_nodes %d\n", intFromMap(stats, "fraud_graph_nodes"))
	fmt.Fprintf(w, "# HELP finix_ledger_entries Double-entry ledger entries\n")
	fmt.Fprintf(w, "# TYPE finix_ledger_entries gauge\n")
	fmt.Fprintf(w, "finix_ledger_entries %d\n", intFromMap(stats, "ledger_entries"))
	fmt.Fprintf(w, "# HELP finix_outbox_pending Pending outbox events\n")
	fmt.Fprintf(w, "# TYPE finix_outbox_pending gauge\n")
	fmt.Fprintf(w, "finix_outbox_pending %d\n", intFromMap(stats, "outbox_pending"))
	fmt.Fprintf(w, "# HELP finix_sse_subscribers Active SSE subscribers\n")
	fmt.Fprintf(w, "# TYPE finix_sse_subscribers gauge\n")
	fmt.Fprintf(w, "finix_sse_subscribers %d\n", intFromMap(stats, "sse_subscribers"))
}

func intFromMap(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		}
	}
	return 0
}

// corsMiddleware builds the CORS handler for a specific allow-list. The list is
// per-router instance state (passed in), not package-global, so constructing two
// routers (e.g. in tests) never accumulates or shares origins.
// originMatchesPortWildcard supports one pattern form in CORS_ALLOWED_ORIGINS:
// a trailing ":*" that matches any PORT on an otherwise exact scheme+host, e.g.
//
//	http://localhost:*   matches http://localhost:5599, http://localhost:8080
//	                     but NOT http://localhost.evil.com or https://localhost:1
//
// This exists because `flutter run -d chrome` picks a random port each launch,
// which an exact-match allow-list cannot express. The host and scheme are still
// compared exactly, so this never widens the policy to another origin — only to
// other ports of a host that was already trusted.
func originMatchesPortWildcard(pattern, origin string) bool {
	prefix, ok := strings.CutSuffix(pattern, ":*")
	if !ok {
		return false
	}
	rest, ok := strings.CutPrefix(strings.ToLower(origin), strings.ToLower(prefix))
	if !ok {
		return false
	}
	// Everything after the host must be exactly ":<digits>" — no path, no
	// userinfo, and no additional host labels.
	if !strings.HasPrefix(rest, ":") {
		return false
	}
	port := rest[1:]
	if port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	// Snapshot so later mutation of the caller's slice can't change policy.
	origins := append([]string(nil), allowed...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Classify the origin. Credentialed CORS (Allow-Credentials: true) is only
			// ever paired with an EXACT allow-list match — never with "*" or reflected
			// arbitrary origins, which would defeat the same-origin protection CORS
			// exists to provide. A configured "*" still permits the request, but as an
			// anonymous (non-credentialed) cross-origin response only.
			explicit := false // exact allow-list match → safe to allow credentials
			wildcard := false // "*" configured → allow, but never with credentials
			for _, ao := range origins {
				if ao == "*" {
					wildcard = true
					continue
				}
				if strings.EqualFold(ao, origin) {
					explicit = true
					break
				}
				if originMatchesPortWildcard(ao, origin) {
					explicit = true
					break
				}
			}
			if !explicit && !wildcard {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Internal-Token, X-Admin-Token, X-Biometric-Challenge-Id, X-Biometric-Challenge, X-Device-Fingerprint, X-Device-Trusted")
			w.Header().Set("Access-Control-Max-Age", "300")
			if explicit {
				// Reflect the vetted origin AND allow credentials (cookies/Authorization).
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				// Wildcard policy: anonymous access only. Per the CORS spec, "*" and
				// Allow-Credentials:true are mutually exclusive, so credentials are
				// deliberately NOT enabled here.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// maxBodyBytes caps the request body: http.MaxBytesReader makes a subsequent
// Decode return an error once the limit is exceeded (and signals the server to
// close the connection), so a huge or slow-drip payload can't exhaust memory.
func maxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// csrfDefense rejects state-changing requests a browser marks as cross-site
// (L-7). This API authenticates via the Authorization: Bearer header, which a
// cross-site form/image/script cannot set, so it is already structurally
// CSRF-resistant; this adds defence-in-depth using Fetch Metadata. Non-browser
// clients (the mobile app) do not send Sec-Fetch-Site and are unaffected.
func csrfDefense(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
				http.Error(w, "cross-site state-changing request rejected", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	// A mobile client (Flutter) may send fields the server struct
	// doesn't yet have after an app update. DisallowUnknownFields would 400
	// every such request and break the client. We trust client input and
	// validate server-side instead. Enable strict mode only if explicitly
	// requested via DECODE_STRICT_FIELDS (e.g. contract tests).
	if strings.EqualFold(os.Getenv("DECODE_STRICT_FIELDS"), "true") {
		decoder.DisallowUnknownFields()
	}
	return decoder.Decode(dst)
}

func userIDFromRequest(r *http.Request) (string, error) {
	uid, ok := apimw.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(uid) == "" {
		return "", errors.New("unauthorized")
	}
	return uid, nil
}

// --- PQC Kyber Handlers ---
func (api *API) pqcInit(w http.ResponseWriter, r *http.Request) {
	if api.pqc == nil {
		writeError(w, http.StatusServiceUnavailable, "PQC service is not available")
		return
	}
	keyInfo := api.pqc.GetKeyInfo()
	writeJSON(w, http.StatusOK, map[string]any{
		"publicKey":  api.pqc.GetPublicKeyB64(),
		"algorithm":  keyInfo.Algorithm,
		"keyId":      keyInfo.KeyID,
		"keyInfo":    keyInfo,
		"sessionTTL": 300,
	})
}

func (api *API) pqcKeyInfo(w http.ResponseWriter, r *http.Request) {
	if api.pqc == nil {
		writeError(w, http.StatusServiceUnavailable, "PQC service is not available")
		return
	}
	writeJSON(w, http.StatusOK, api.pqc.GetKeyInfo())
}

func (api *API) pqcEncapsulate(w http.ResponseWriter, r *http.Request) {
	if api.pqc == nil {
		writeError(w, http.StatusServiceUnavailable, "PQC service is not available")
		return
	}
	var req struct {
		Ciphertext string `json:"ciphertext"`
		// Optional: the keyId the client encapsulated to (from /pqc/init or
		// /pqc/key-info). Lets the server pick the just-rotated previous key during
		// the grace window; omitted → current key.
		KeyID string `json:"keyId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sessionID, _, err := api.pqc.DecapsulateWithKey(req.Ciphertext, req.KeyID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "decapsulation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"sessionId": sessionID,
		"expiresIn": 300,
		"message":   "Use X-PQC-Session header with this sessionId for encrypted requests. Session expires in 5 minutes.",
	})
}

// --- Dilithium Signature Handlers ---
func (api *API) dilithiumPublicKey(w http.ResponseWriter, r *http.Request) {
	if api.dilithium == nil {
		writeError(w, http.StatusServiceUnavailable, "Dilithium service is not available")
		return
	}
	// keyInfo is the redacted view (no private key). Expose only the public
	// signing material the client needs to verify signatures.
	keyInfo := api.dilithium.GetCurrentKeyInfo()
	writeJSON(w, http.StatusOK, map[string]any{
		"publicKey": api.dilithium.GetPublicKeyB64(),
		"algorithm": keyInfo.Algorithm,
		"keyId":     keyInfo.KeyID,
	})
}

func (api *API) register(w http.ResponseWriter, r *http.Request) {
	var req platform.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMobile(req.Mobile); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateOptionalEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.Register(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) verifyEKYC(w http.ResponseWriter, r *http.Request) {
	var req platform.EKYCRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLast4("panLast4", req.PanLast4); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateLast4("aadhaarLast4", req.AadhaarLast4); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.VerifyEKYC(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) registerBiometric(w http.ResponseWriter, r *http.Request) {
	var req platform.BiometricSetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.RegisterBiometric(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) loginChallenge(w http.ResponseWriter, r *http.Request) {
	var req platform.LoginChallengeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.CreateChallenge(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) loginVerify(w http.ResponseWriter, r *http.Request) {
	var req platform.LoginVerifyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.VerifyLogin(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) pinLogin(w http.ResponseWriter, r *http.Request) {
	var req platform.PinLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The mobile client sends its device fingerprint as a header on every
	// request rather than in the login body; fall back to it so the device-bound
	// check still has a value.
	if strings.TrimSpace(req.DeviceIDFingerprint) == "" {
		req.DeviceIDFingerprint = strings.TrimSpace(r.Header.Get("X-Device-Fingerprint"))
	}
	res, err := api.svc.LoginWithPIN(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// refreshToken exchanges an expiring/expired-within-grace JWT for a fresh one.
// The token may come from the Authorization: Bearer header or a JSON body.
func (api *API) refreshToken(w http.ResponseWriter, r *http.Request) {
	token := ""
	if h := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		token = strings.TrimSpace(h[7:])
	}
	if token == "" {
		var body struct {
			Token string `json:"token"`
		}
		_ = decodeJSON(r, &body)
		token = strings.TrimSpace(body.Token)
	}
	deviceFP := strings.TrimSpace(r.Header.Get("X-Device-Fingerprint"))

	res, err := api.svc.RefreshToken(token, deviceFP)
	if err != nil {
		// A blown grace window / device mismatch is a hard re-login signal.
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": err.Error(), "code": "reauth_required",
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) pinSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PIN    string `json:"pin"`
		UserID string `json:"userId,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Authenticated PIN change (session present) — the normal path.
	if uid, err := userIDFromRequest(r); err == nil {
		if err := api.svc.SetPIN(uid, req.PIN); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "pin_set"})
		return
	}

	// Onboarding: register -> eKYC -> biometric -> set PIN -> login, so this call
	// arrives before any token exists. SetInitialPIN only succeeds when the
	// account has no PIN yet AND the request comes from the device that
	// registered it, so this is not a general unauthenticated PIN reset.
	uid := strings.TrimSpace(req.UserID)
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized: supply a bearer token, or userId during onboarding")
		return
	}
	if err := api.svc.SetInitialPIN(uid, req.PIN, r.Header.Get("X-Device-Fingerprint")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pin_set"})
}

func (api *API) dummyBankVerifyUPI(w http.ResponseWriter, r *http.Request) {
	upiID := r.URL.Query().Get("upiId")
	if upiID == "" {
		writeError(w, http.StatusBadRequest, "upiId query parameter is required")
		return
	}
	res, err := api.svc.DummyBankVerifyUPI(upiID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) dummyBankVerifyIFSC(w http.ResponseWriter, r *http.Request) {
	ifsc := r.URL.Query().Get("ifsc")
	accountNumber := r.URL.Query().Get("accountNumber")
	res, err := api.svc.DummyBankVerifyIFSC(ifsc, accountNumber)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) dummyBankBalance(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("accountId")
	res, err := api.svc.DummyBankBalance(accountID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) oauthToken(w http.ResponseWriter, r *http.Request) {
	var req platform.TokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if api.svc == nil {
		writeError(w, http.StatusInternalServerError, "service not initialized")
		return
	}
	res, err := api.svc.ExchangeOAuthToken(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		CodeChallenge string `json:"code_challenge"`
	}
	// C-1 fix: do NOT silently ignore a decode error — that path issued an auth code
	// with an empty PKCE challenge, which verifyPKCE then accepted unconditionally.
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// C-1 fix: PKCE is mandatory. Reject any authorize request without a challenge.
	if strings.TrimSpace(req.CodeChallenge) == "" {
		writeError(w, http.StatusBadRequest, "code_challenge is required (PKCE mandatory)")
		return
	}
	code := api.svc.GenerateOAuthCode(uid, req.CodeChallenge)
	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_code": code,
		"expires_in":         "300",
	})
}

func (api *API) stepupChallenge(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	challengeID, challenge, err := api.svc.CreateStepUpChallenge(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"challengeId": challengeID,
		"challenge":   challenge,
	})
}

func (api *API) otpGenerate(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := api.svc.GenerateOTP(uid); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// H-1 fix: never return the OTP in the API response. It must only be delivered
	// via SMS/TOTP. Returning it here exposes it to proxy logs and monitoring.
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "generated",
		"note":   "OTP sent via SMS.",
	})
}

func (api *API) authProfile(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.AuthProfile(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res.SanitizeForAPI()
	writeJSON(w, http.StatusOK, res)
}

func (api *API) updateAuthProfile(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.UpdateAuthProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.UpdateAuthProfile(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) kycProfile(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.KYCProfile(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res.SanitizeForAPI()
	writeJSON(w, http.StatusOK, res)
}

func (api *API) upsertKYCProfile(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.UpsertKYCProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.UpsertKYCProfile(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) dashboard(w http.ResponseWriter, r *http.Request) {
	span := platform.StartSpan("GET /v1/dashboard")
	defer span.Finish()
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	span.SetTag("user_id", uid)
	res, err := api.svc.Dashboard(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) linkAccount(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.LinkAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.LinkAccount(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) listAccounts(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Accounts(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) updateAccount(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	accountID := chi.URLParam(r, "accountID")
	var req platform.UpdateAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.UpdateAccount(uid, accountID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) createBeneficiary(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.CreateBeneficiaryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.CreateBeneficiary(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) listBeneficiaries(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Beneficiaries(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) approveBeneficiary(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	beneficiaryID := chi.URLParam(r, "beneficiaryID")
	var req platform.ApproveBeneficiaryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.ApproveBeneficiary(uid, beneficiaryID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) aggregatorStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.AggregatorStatus(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) syncAggregator(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.SyncAggregatorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.SyncAggregator(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) predictBehaviour(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.PredictBehaviour(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) recommendInvestments(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.RecommendInvestments(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) generateSyntheticDataset(w http.ResponseWriter, r *http.Request) {
	var req platform.SyntheticDatasetSpec
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	res := api.svc.GenerateSyntheticDataset(req)
	writeJSON(w, http.StatusOK, res)
}

func (api *API) bankAPIReadiness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.svc.BankAPIReadiness())
}

func (api *API) connectBankAPI(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.BankConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.ConnectBankAPI(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) bankConnection(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.BankConnection(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) ingestBankEvent(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.BankTransactionEvent
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.IngestBankEvent(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) realtimeDetections(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
			limit = parsed
		}
	}
	res, err := api.svc.RealtimeDetections(uid, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) syntheticBankEventStream(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.SyntheticBankStreamRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	res, err := api.svc.StreamSyntheticBankEvents(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// idempotencyKeyOrGenerate returns the client's Idempotency-Key header, or a
// generated fallback UUID when the header is absent. This keeps the header
// OPTIONAL: a request without it still gets a unique key and succeeds, instead
// of being rejected with 400. Clients that want cross-retry deduplication must
// send their own stable key.
func idempotencyKeyOrGenerate(r *http.Request) string {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = uuid.NewString()
		slog.Debug("no Idempotency-Key header supplied; generated fallback",
			"key", key, "path", r.URL.Path)
	}
	return key
}

func (api *API) initiateTransaction(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.InitiateTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ValidateTransactionRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	idempotencyKey := idempotencyKeyOrGenerate(r)
	res, err := api.svc.InitiateTransaction(uid, idempotencyKey, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) overrideTransaction(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.OverrideRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.OverrideTransaction(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) transactionHistory(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.svc.TransactionHistory(uid))
}

func (api *API) initiatePayment(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.InitiatePaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePaymentAmount(req.AmountPaise); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.InitiatePayment(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) listPayments(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.ListPayments(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) getPayment(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	paymentID := chi.URLParam(r, "paymentID")
	res, err := api.svc.GetPayment(uid, paymentID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) verifyPaymentReceipt(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	paymentID := chi.URLParam(r, "paymentID")
	var req platform.VerifyReceiptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.VerifyPaymentReceipt(uid, paymentID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) healthScore(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.HealthScore(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) createGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.GoalCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.CreateGoal(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) listGoals(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api.svc.Goals(uid))
}

func (api *API) goalByID(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	res, err := api.svc.GoalByID(uid, goalID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) contributeGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	var req platform.GoalContributionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// L-9: carry the client's Idempotency-Key into the service so a retry can't
	// double-debit the goal contribution. Optional: a fallback UUID is generated
	// when the header is absent so the request still succeeds.
	req.IdempotencyKey = idempotencyKeyOrGenerate(r)
	res, err := api.svc.ContributeGoal(uid, goalID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) closeGoal(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	goalID := chi.URLParam(r, "goalID")
	res, err := api.svc.CloseGoal(uid, goalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) portfolioSummary(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.PortfolioSummary(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) portfolioInvestments(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Investments(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) portfolioInsurance(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Insurance(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Return the aggregate object ({totalLifeCoverPaise, totalHealthCoverPaise,
	// policies:[...]}) rather than a bare array: the mobile client decodes this
	// endpoint as a map and reads the cover totals directly.
	writeJSON(w, http.StatusOK, platform.BuildInsurancePortfolio(res))
}

func (api *API) portfolioLoans(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Loans(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) runSimulation(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.SimulationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.RunSimulation(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) insightsFeed(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.InsightsFeed(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) insightsMarket(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.MarketSnapshot(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) insightsPersona(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Persona(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) securityHealth(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.SecurityStatus(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) scanSMS(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.SMSScanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.ScanSMS(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) emergencyFreeze(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.FreezeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.EmergencyFreeze(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) unfreeze(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.UnfreezeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.Unfreeze(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) reportFraud(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.FraudReportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.ReportFraud(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) taxDashboard(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.TaxDashboard(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) taxRegimeCompare(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.RegimeComparison(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) taxDeductions(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Deductions(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) taxCapitalGains(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.CapitalGains(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) chat(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.ChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.Chat(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) profile(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.Profile(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) setConsent(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	consentType := chi.URLParam(r, "consentType")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	granted, present, err := consentBooleanFromPayload(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !present {
		writeError(w, http.StatusBadRequest, "accepted/granted field is required")
		return
	}

	res, err := api.svc.SetConsent(uid, consentType, granted)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.EqualFold(strings.TrimSpace(consentType), "privacy_policy") {
		consentID := firstStringValue(res, "consent_id")
		if consentID == "" {
			consentID = "cns_pp_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
		}

		txHash := firstStringValue(res, "blockchain_tx_hash")
		if txHash == "" {
			txHash = syntheticConsentTxHash(consentID + ":" + uid)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"consent_recorded":   granted,
			"consent_id":         consentID,
			"blockchain_tx_hash": txHash,
			"next_review_date":   nil,
			"message":            "Your consent has been recorded on our blockchain ledger.",
			"type":               consentType,
			"granted":            granted,
		})
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (api *API) setPublicPrivacyConsent(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	accepted, present, err := consentBooleanFromPayload(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !present || !accepted {
		writeError(w, http.StatusBadRequest, "accepted/granted must be true for privacy policy consent")
		return
	}

	consentType := "privacy_policy"
	consentID := "cns_pp_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	txHash := ""

	if token := bearerTokenFromRequest(r); token != "" {
		deviceFP := strings.TrimSpace(r.Header.Get("X-Device-Fingerprint"))
		if uid, ok := api.svc.AuthenticateToken(token, deviceFP); ok {
			res, setErr := api.svc.SetConsent(uid, consentType, true)
			if setErr == nil {
				if v := firstStringValue(res, "consent_id"); v != "" {
					consentID = v
				}
				if v := firstStringValue(res, "blockchain_tx_hash"); v != "" {
					txHash = v
				}
			}
		}
	}

	if txHash == "" {
		seed := strings.TrimSpace(firstStringValue(req, "device_id")) + ":" +
			strings.TrimSpace(firstStringValue(req, "accepted_at")) + ":" + consentID
		txHash = syntheticConsentTxHash(seed)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"consent_recorded":   true,
		"consent_id":         consentID,
		"blockchain_tx_hash": txHash,
		"next_review_date":   nil,
		"message":            "Your consent has been recorded on our blockchain ledger.",
	})
}

func (api *API) setNudgePreference(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req struct {
		Preference string `json:"preference"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.SetNudgePreference(uid, req.Preference)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) notificationSettings(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.NotificationSettings(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) submitFeedback(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var req platform.FeedbackRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := api.svc.SubmitFeedback(uid, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (api *API) auditLogs(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.AuditLogs(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) auditIntegrity(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	res, err := api.svc.AuditLogIntegrity(uid)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (api *API) ledgerEntries(w http.ResponseWriter, r *http.Request) {
	uid, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, api.svc.LedgerEntries(uid, limit))
}

func (api *API) registerExpandedAuthedRoutes(v1 chi.Router) {
	v1.Post("/auth/sim-bind/reverify", api.expandedEndpoint("auth_sim_bind_reverify", true, true))
	v1.Get("/auth/kin", api.expandedEndpoint("auth_kin", true, false))

	v1.With(apimw.RequireOTPVerification(api.svc.VerifyOTPToken)).Post("/accounts/{accountID}/upi-pin/set", api.expandedEndpoint("upi_pin_set", true, true, "accountID"))
	v1.With(
		apimw.RequireOTPVerification(api.svc.VerifyOTPToken),
		apimw.RequireBiometricChallenge(60*time.Second, api.svc.VerifyBiometricChallenge),
	).Post("/accounts/{accountID}/upi-pin/reset", api.expandedEndpoint("upi_pin_reset", true, true, "accountID"))

	v1.Get("/dashboard/net-worth/history", api.expandedEndpoint("dashboard_net_worth_history", true, false))
	v1.Get("/dashboard/market-snapshot", api.expandedEndpoint("dashboard_market_snapshot", true, false))
	v1.Get("/security/freeze-status", api.expandedEndpoint("security_freeze_status", true, false))

	v1.Get("/transactions/{txID}/cooling-off-status", api.expandedEndpoint("tx_cooling_off_status", true, false, "txID"))
	v1.Post("/transactions/{txID}/schedule", api.expandedEndpoint("tx_schedule", true, false, "txID"))
	v1.Get("/transactions/{txID}/risk-score", api.expandedEndpoint("tx_risk_score", true, false, "txID"))
	v1.Post("/beneficiaries/{beneficiaryID}/risk-check", api.expandedEndpoint("beneficiary_risk_check", true, true, "beneficiaryID"))

	v1.Post("/goals/{goalID}/recalculate", api.expandedEndpoint("goals_recalculate", true, false, "goalID"))
	v1.Get("/goals/archived", api.expandedEndpoint("goals_archived", true, false))
	v1.Get("/goals/{goalID}/milestones", api.expandedEndpoint("goals_milestones", true, false, "goalID"))
	v1.Get("/goals/{goalID}/history", api.expandedEndpoint("goals_history", true, false, "goalID"))
	v1.Post("/goals/market-impact-recalculate", api.expandedEndpoint("goals_market_impact_recalculate", true, false))

	v1.Get("/health-score/pillars", api.expandedEndpoint("health_pillars", true, false))
	v1.Get("/health-score/history", api.expandedEndpoint("health_history", true, false))
	v1.Post("/health-score/simulate", api.expandedEndpoint("health_simulate", true, true))

	v1.Get("/portfolio/investments/{holdingID}", api.expandedEndpoint("investment_detail", true, false, "holdingID"))
	// Investment orders >= ₹25,000 require a fresh biometric challenge (§9.4A.4).
	v1.With(
		apimw.RequireBiometricAboveAmount(2500000, 60*time.Second, api.svc.VerifyBiometricChallenge),
		apimw.EnforceCoolingOff("", api.svc.CoolingOffState),
	).Post("/portfolio/investments/order", api.expandedEndpoint("investment_order", true, true))
	v1.Post("/portfolio/investments/sip/start", api.expandedEndpoint("investment_sip_start", true, true))
	v1.With(apimw.RequireOTPVerification(api.svc.VerifyOTPToken)).Post("/portfolio/investments/sip/{sipID}/pause", api.expandedEndpoint("investment_sip_pause", true, true, "sipID"))
	v1.With(apimw.RequireOTPVerification(api.svc.VerifyOTPToken)).Post("/portfolio/investments/sip/{sipID}/stop", api.expandedEndpoint("investment_sip_stop", true, true, "sipID"))
	v1.With(apimw.RequireOTPVerification(api.svc.VerifyOTPToken)).Patch("/portfolio/investments/sip/{sipID}", api.expandedEndpoint("investment_sip_modify", true, true, "sipID"))
	v1.Get("/portfolio/investments/liquidation-check", api.expandedEndpoint("investment_liquidation_check", true, false))
	v1.Get("/portfolio/investments/rebalance-suggestion", api.expandedEndpoint("investment_rebalance_suggestion", true, false))
	v1.Get("/portfolio/investments/tax-loss-harvest", api.expandedEndpoint("investment_tax_loss_harvest", true, false))
	v1.Post("/security/url-scan", api.expandedEndpoint("security_url_scan", true, true))

	v1.Post("/portfolio/insurance", api.expandedEndpoint("insurance_add", true, true))
	v1.Get("/portfolio/insurance/{policyID}", api.expandedEndpoint("insurance_detail", true, false, "policyID"))
	v1.Delete("/portfolio/insurance/{policyID}", api.expandedEndpoint("insurance_delete", true, false, "policyID"))
	v1.With(
		apimw.RequireOTPVerification(api.svc.VerifyOTPToken),
		apimw.RequireBiometricChallenge(60*time.Second, api.svc.VerifyBiometricChallenge),
	).Patch("/portfolio/insurance/{policyID}/nominee", api.expandedEndpoint("insurance_nominee_change", true, true, "policyID"))
	v1.Get("/portfolio/insurance/gap-analysis", api.expandedEndpoint("insurance_gap_analysis", true, false))
	v1.Get("/portfolio/insurance/protection-score", api.expandedEndpoint("insurance_protection_score", true, false))
	v1.Post("/portfolio/insurance/verify-document", api.expandedEndpoint("insurance_verify_document", true, true))
	v1.Post("/portfolio/insurance/{policyID}/claim", api.expandedEndpoint("insurance_claim_create", true, true, "policyID"))
	v1.Get("/portfolio/insurance/{policyID}/claims", api.expandedEndpoint("insurance_claims_list", true, false, "policyID"))
	v1.Get("/portfolio/insurance/{policyID}/claims/{claimID}", api.expandedEndpoint("insurance_claim_detail", true, false, "policyID", "claimID"))
	v1.Patch("/portfolio/insurance/{policyID}/claims/{claimID}", api.expandedEndpoint("insurance_claim_update", true, true, "policyID", "claimID"))
	v1.Get("/portfolio/insurance/network-hospitals", api.expandedEndpoint("insurance_network_hospitals", true, false))
	v1.Post("/portfolio/insurance/{policyID}/premium-payment", api.expandedEndpoint("insurance_premium_payment", true, true, "policyID"))

	v1.Get("/portfolio/loans/{loanID}", api.expandedEndpoint("loan_detail", true, false, "loanID"))
	v1.Post("/portfolio/loans/{loanID}/prepayment-simulate", api.expandedEndpoint("loan_prepayment_simulate", true, true, "loanID"))
	v1.Get("/portfolio/loans/{loanID}/foreclosure-check", api.expandedEndpoint("loan_foreclosure_check", true, false, "loanID"))
	v1.Get("/portfolio/loans/credit-utilisation", api.expandedEndpoint("loan_credit_utilisation", true, false))
	v1.Get("/portfolio/loans/bnpl", api.expandedEndpoint("loan_bnpl", true, false))
	v1.Get("/portfolio/loans/optimisation-strategy", api.expandedEndpoint("loan_optimisation_strategy", true, false))
	v1.Post("/portfolio/loans/rate-update-notify", api.expandedEndpoint("loan_rate_update_notify", true, true))
	v1.Get("/portfolio/loans/{loanID}/repayment-integrity", api.expandedEndpoint("loan_repayment_integrity", true, false, "loanID"))
	v1.Post("/portfolio/loans/verify-payment-link", api.expandedEndpoint("loan_verify_payment_link", true, true))
	v1.Get("/portfolio/loans/debt-free-date", api.expandedEndpoint("loan_debt_free_date", true, false))

	v1.Get("/simulations/history", api.expandedEndpoint("simulations_history", true, false))
	v1.Get("/simulations/types", api.expandedEndpoint("simulations_types", true, false))

	v1.Post("/insights/persona/contest", api.expandedEndpoint("insights_persona_contest", true, true))
	v1.Post("/insights/feed/{insightID}/feedback", api.expandedEndpoint("insights_feedback", true, true, "insightID"))
	v1.Get("/insights/market/brief", api.expandedEndpoint("insights_market_brief", true, false))
	v1.Get("/insights/market/sector-heatmap", api.expandedEndpoint("insights_sector_heatmap", true, false))
	v1.Get("/insights/market/ipo-calendar", api.expandedEndpoint("insights_ipo_calendar", true, false))
	v1.Get("/insights/market/fund-compare", api.expandedEndpoint("insights_fund_compare", true, false))
	v1.Get("/insights/market/rate-history", api.expandedEndpoint("insights_rate_history", true, false))
	v1.Post("/insights/market/panic-check", api.expandedEndpoint("insights_panic_check", true, true))
	v1.Get("/insights/market/news-sources", api.expandedEndpoint("insights_news_sources", true, false))
	v1.Get("/insights/nudge-log", api.expandedEndpoint("insights_nudge_log", true, false))
	v1.Post("/aiml/behaviour/persona/contest", api.expandedEndpoint("aiml_behaviour_persona_contest", true, true))

	// NOTE: /security/emergency-contacts (GET/POST/DELETE) and /security/tips are
	// registered with their real handlers in NewRouter (see api.emergencyContacts,
	// api.addEmergencyContact, api.deleteEmergencyContact, api.securityTips). They
	// must NOT be re-registered here: chi's last-registration-wins would shadow the
	// real handlers with these generic expandedEndpoint stubs.
	v1.Get("/security/bank-contacts", api.expandedEndpoint("security_bank_contacts", true, false))
	v1.Post("/security/report-fraud/forward-nccp", api.expandedEndpoint("security_forward_nccp", true, true))
	v1.Post("/security/emergency-freeze/test", api.expandedEndpoint("security_emergency_freeze_test", true, false))

	v1.Get("/tax/advance-tax-schedule", api.expandedEndpoint("tax_advance_schedule", true, false))
	v1.Post("/tax/capital-gains/pre-sale-check", api.expandedEndpoint("tax_pre_sale_check", true, true))
	v1.Get("/tax/deductions/tracker", api.expandedEndpoint("tax_deductions_tracker", true, false))
	v1.Get("/tax/literacy/spending-breakdown", api.expandedEndpoint("tax_literacy_spending", true, false))

	v1.Get("/chatbot/history", api.expandedEndpoint("chatbot_history", true, false))
	v1.Post("/chatbot/query/voice", api.expandedEndpoint("chatbot_query_voice", true, true))
	v1.Delete("/chatbot/history", api.expandedEndpoint("chatbot_history_delete", true, false))

	v1.Post("/settings/data-export", api.expandedEndpoint("settings_data_export_create", true, true))
	v1.Get("/settings/data-export/{exportID}", api.expandedEndpoint("settings_data_export_status", true, false, "exportID"))
	v1.Get("/settings/data-sharing-log", api.expandedEndpoint("settings_data_sharing_log", true, false))
	v1.Get("/settings/data-retention", api.expandedEndpoint("settings_data_retention", true, false))
	v1.Get("/settings/consent/history", api.expandedEndpoint("settings_consent_history", true, false))
	v1.Get("/settings/grievance-contact", api.expandedEndpoint("settings_grievance_contact", true, false))
	v1.Get("/settings/transparency-report", api.expandedEndpoint("settings_transparency_report", true, false))
	v1.Get("/settings/ethical-charter", api.expandedEndpoint("settings_ethical_charter", true, false))

	v1.Post("/audit/logs/export", api.expandedEndpoint("audit_logs_export", true, false))
	v1.Get("/audit/logs/integrity-proof", api.expandedEndpoint("audit_logs_integrity_proof", true, false))

	v1.Get("/blockchain/news-whitelist", api.expandedEndpoint("blockchain_news_whitelist", true, false))

	v1.Delete("/beneficiaries/{beneficiaryID}", api.expandedEndpoint("beneficiary_delete", true, false, "beneficiaryID"))
	v1.Get("/beneficiaries/{beneficiaryID}", api.expandedEndpoint("beneficiary_detail", true, false, "beneficiaryID"))
	v1.Get("/beneficiaries/recent", api.expandedEndpoint("beneficiaries_recent", true, false))
}

func (api *API) expandedEndpoint(endpointID string, requireAuth bool, requireBody bool, pathParamKeys ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := ""
		if requireAuth {
			resolvedUID, err := userIDFromRequest(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}
			uid = resolvedUID
		}

		body := map[string]any{}
		if requireBody {
			if err := decodeJSON(r, &body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		} else if r.Body != nil && r.ContentLength != 0 {
			if err := decodeJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}

		pathParams := make(map[string]string, len(pathParamKeys))
		for _, key := range pathParamKeys {
			pathParams[key] = chi.URLParam(r, key)
		}

		query := map[string]string{}
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				query[key] = values[0]
			}
		}

		// L-6 fix: forward only an explicit allow-list of business headers to the
		// expanded-endpoint service. A blacklist silently forwarded everything else
		// — including client-spoofable proxy headers (X-Forwarded-For) and any new
		// sensitive header added later — so business logic could be influenced by
		// forged headers. Whitelisting fails closed.
		headers := map[string]string{}
		for key, values := range r.Header {
			if len(values) == 0 {
				continue
			}
			if _, ok := forwardableHeaders[http.CanonicalHeaderKey(key)]; ok {
				headers[key] = values[0]
			}
		}

		res, err := api.svc.HandleExpandedEndpoint(uid, endpointID, platform.EndpointInput{
			PathParams: pathParams,
			Query:      query,
			Body:       body,
			Headers:    headers,
		})
		if err != nil {
			writeError(w, statusForExpandedError(err), err.Error())
			return
		}

		writeJSON(w, http.StatusOK, res)
	}
}

func statusForExpandedError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unauthorized"), strings.Contains(message, "token"):
		return http.StatusUnauthorized
	case strings.Contains(message, "forbidden"):
		return http.StatusForbidden
	case strings.Contains(message, "not found"):
		return http.StatusNotFound
	case strings.Contains(message, "locked"), strings.Contains(message, "cooling-off"):
		return http.StatusLocked
	default:
		return http.StatusBadRequest
	}
}

func consentBooleanFromPayload(payload map[string]any) (bool, bool, error) {
	if payload == nil {
		return false, false, nil
	}

	if raw, ok := payload["accepted"]; ok {
		v, err := strictBool(raw)
		return v, true, err
	}

	if raw, ok := payload["granted"]; ok {
		v, err := strictBool(raw)
		return v, true, err
	}

	return false, false, nil
}

func strictBool(value any) (bool, error) {
	v, ok := value.(bool)
	if !ok {
		return false, errors.New("accepted/granted must be a boolean")
	}
	return v, nil
}

func firstStringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	text, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func bearerTokenFromRequest(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func syntheticConsentTxHash(seed string) string {
	if strings.TrimSpace(seed) == "" {
		seed = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	sum := sha256.Sum256([]byte(seed))
	return "0x" + hex.EncodeToString(sum[:])
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func tokenOrDefault(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "change-this-immediately"
	}
	return hex.EncodeToString(bytes)
}

// validateRisk handles POST /v1/internal/risk-validation
// Called by Python RAG validation worker to submit validation verdicts
func (api *API) validateRisk(w http.ResponseWriter, r *http.Request) {
	var req platform.ValidateRiskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate required fields
	if req.TxID == "" {
		writeError(w, http.StatusBadRequest, "tx_id is required")
		return
	}
	if req.Verdict.SuggestedLevel == "" {
		writeError(w, http.StatusBadRequest, "suggested_level is required")
		return
	}
	if req.Verdict.RecommendedAction == "" {
		writeError(w, http.StatusBadRequest, "recommended_action is required")
		return
	}

	// Call service to apply validation
	ctx := r.Context()
	resp, err := api.svc.ValidateRisk(ctx, req.TxID, req)
	if err != nil {
		// Check if it's a cooling-off expired error
		if strings.Contains(err.Error(), "cooling-off") || strings.Contains(err.Error(), "expired") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applied":    resp.Applied,
		"tx_id":      req.TxID,
		"new_status": resp.NewStatus,
		"old_status": resp.OldStatus,
		"message":    "Risk validation applied successfully",
	})
}

// pqcEnforced reports whether authenticated routes must carry a valid

// pqcEnforced reports whether authenticated routes must carry a valid
// X-PQC-Session header. Off by default so tooling/tests and clients that have
// not yet completed the Kyber handshake keep working; set FINIX_REQUIRE_PQC to
// true/1/yes in production once clients establish PQC sessions.
func pqcEnforced() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_REQUIRE_PQC"))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
