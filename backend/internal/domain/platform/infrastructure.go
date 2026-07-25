package platform

import (
	"context"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// SECRETS MANAGER ABSTRACTION
// Production: HashiCorp Vault, AWS Secrets Manager, or Azure Key Vault.
// Development: environment variables with fallback.
// ALL secrets are server-side only — the frontend never sees them.
// ============================================================

type SecretsBackend string

const (
	SecretsBackendEnv   SecretsBackend = "env"
	SecretsBackendVault SecretsBackend = "vault"
)

type SecretsManager struct {
	mu      sync.RWMutex
	backend SecretsBackend
	cache   map[string]string
}

var sharedSecretsManager *SecretsManager

func NewSecretsManager() *SecretsManager {
	backend := SecretsBackend(os.Getenv("FINIX_SECRETS_BACKEND"))
	if backend == "" {
		backend = SecretsBackendEnv
	}
	sm := &SecretsManager{
		backend: backend,
		cache:   make(map[string]string),
	}
	sharedSecretsManager = sm
	return sm
}

func (sm *SecretsManager) GetSecret(key string) string {
	sm.mu.RLock()
	if v, ok := sm.cache[key]; ok {
		sm.mu.RUnlock()
		return v
	}
	sm.mu.RUnlock()

	var value string
	switch sm.backend {
	case SecretsBackendVault:
		value = sm.fetchFromVault(key)
	default:
		value = os.Getenv(key)
	}

	// Only cache non-empty values to avoid caching missing secrets
	if value != "" {
		sm.mu.Lock()
		sm.cache[key] = value
		sm.mu.Unlock()
	}
	return value
}

func (sm *SecretsManager) fetchFromVault(key string) string {
	// Stub: in production, calls HashiCorp Vault API via vault_addr + vault_token
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		// M-16 fix: do NOT silently fall back to plaintext env vars. An operator who
		// selected the vault backend would otherwise unknowingly run on env secrets.
		// Return empty so the caller treats the secret as unavailable (fail loud).
		log.Printf("[SECRETS] FATAL CONFIG: vault backend selected but VAULT_ADDR is not set — refusing to fall back to env for key %q", key)
		return ""
	}
	// In production implementation: HTTP GET vaultAddr/v1/secret/data/finix/{key}
	return ""
}

func GetSecret(key string) string {
	if sharedSecretsManager != nil {
		return sharedSecretsManager.GetSecret(key)
	}
	return os.Getenv(key)
}

// ============================================================
// OUTBOX PATTERN
// Ensures events are published even if async DB save fails.
// Events are written to the outbox first, then a background worker
// publishes them (to Kafka/Redis) and marks them as processed.
// ============================================================

type OutboxEvent struct {
	ID        string
	Topic     string
	Payload   string
	CreatedAt time.Time
	Retries   int
	Processed bool
}

type Outbox struct {
	mu     sync.Mutex
	events []*OutboxEvent
}

func NewOutbox() *Outbox {
	return &Outbox{
		events: make([]*OutboxEvent, 0, 1024),
	}
}

func (o *Outbox) Enqueue(topic, payload string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, &OutboxEvent{
		ID:        "obx_" + randomHex(12),
		Topic:     topic,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
}

func (o *Outbox) Dequeue(limit int) []*OutboxEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make([]*OutboxEvent, 0, limit)
	for _, e := range o.events {
		if !e.Processed && len(result) < limit {
			result = append(result, e)
		}
	}
	return result
}

func (o *Outbox) MarkProcessed(eventID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, e := range o.events {
		if e.ID == eventID {
			e.Processed = true
			return
		}
	}
}

func (o *Outbox) MarkFailed(eventID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, e := range o.events {
		if e.ID == eventID {
			e.Retries++
			if e.Retries >= 3 {
				e.Processed = true // move to dead letter (processed=true means skip)
			}
			return
		}
	}
}

func (o *Outbox) PendingCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	count := 0
	for _, e := range o.events {
		if !e.Processed {
			count++
		}
	}
	return count
}

// Purge removes processed events older than retention so the backing slice can't
// grow without bound (M-12). Unprocessed events are always kept for retry.
func (o *Outbox) Purge(retention time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cutoff := time.Now().UTC().Add(-retention)
	kept := o.events[:0]
	for _, e := range o.events {
		if e.Processed && e.CreatedAt.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
	}
	o.events = kept
}

// ============================================================
// CIRCUIT BREAKER
// Protects against cascading failures from unresponsive DB or external services.
// Three states: Closed (normal), Open (failing), HalfOpen (testing recovery).
// ============================================================

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	resetTimeout     time.Duration
	lastFailure      time.Time
	lastStateChange  time.Time
	halfOpenCount    int32 // atomic counter for half-open in-flight requests
}

func NewCircuitBreaker(failureThreshold, successThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		resetTimeout:     resetTimeout,
		lastStateChange:  time.Now().UTC(),
	}
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	cb.mu.Lock()
	switch cb.state {
	case CircuitOpen:
		if time.Since(cb.lastStateChange) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			cb.lastStateChange = time.Now().UTC()
			log.Println("[CIRCUIT] transitioning to half-open")
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	case CircuitHalfOpen:
		// Only allow ONE request at a time in half-open state
		if atomic.LoadInt32(&cb.halfOpenCount) > 0 {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
		atomic.AddInt32(&cb.halfOpenCount, 1)
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Decrement half-open counter if we were in half-open
	if cb.state == CircuitHalfOpen {
		atomic.AddInt32(&cb.halfOpenCount, -1)
	}

	if err != nil {
		cb.failureCount++
		cb.lastFailure = time.Now().UTC()
		if cb.failureCount >= cb.failureThreshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now().UTC()
			log.Printf("[CIRCUIT] opened after %d failures", cb.failureCount)
		}
		// On failure in half-open, go back to open
		if cb.state == CircuitHalfOpen {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now().UTC()
			log.Println("[CIRCUIT] failure in half-open, returning to open")
		}
		return err
	}

	cb.failureCount = 0
	if cb.state == CircuitHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = CircuitClosed
			cb.lastStateChange = time.Now().UTC()
			log.Println("[CIRCUIT] closed — recovery successful")
		}
	}
	return nil
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

var ErrCircuitOpen = &circuitOpenError{}

type circuitOpenError struct{}

func (e *circuitOpenError) Error() string { return "circuit breaker is open" }
