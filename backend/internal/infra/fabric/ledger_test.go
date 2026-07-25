package fabric_test

// Integration test for the Hyperledger Fabric event ledger.
//
// Requires the local network (deploy/fabric/scripts/up.sh) and Postgres. It is
// skipped unless FABRIC_IT=1, so `go test ./...` stays hermetic in CI:
//
//	FABRIC_IT=1 go test ./internal/infra/fabric/...

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"FINIX/backend/internal/domain/blockchain"
	"FINIX/backend/internal/infra/fabric"

	"github.com/jackc/pgx/v5/pgxpool"
)

func requireIT(t *testing.T) {
	t.Helper()
	if os.Getenv("FABRIC_IT") != "1" {
		t.Skip("set FABRIC_IT=1 (with the Fabric network + Postgres up) to run")
	}
}

func testConfig() fabric.Config {
	crypto := os.Getenv("FABRIC_CRYPTO_PATH")
	if crypto == "" {
		crypto = "../../../../deploy/fabric/organizations/peerOrganizations/finix.local"
	}
	return fabric.Config{
		PeerEndpoint: envOr("FABRIC_PEER_ENDPOINT", "localhost:7051"),
		GatewayPeer:  envOr("FABRIC_GATEWAY_PEER", "peer0.finix.local"),
		Channel:      envOr("FABRIC_CHANNEL", "finix-channel"),
		Chaincode:    envOr("FABRIC_CHAINCODE", "finix"),
		MSPID:        envOr("FABRIC_MSP_ID", "FinixMSP"),
		CertPath:     crypto + "/users/User1@finix.local/msp/signcerts",
		KeyDir:       crypto + "/users/User1@finix.local/msp/keystore",
		TLSCertPath:  crypto + "/peers/peer0.finix.local/tls/ca.crt",
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func dial(t *testing.T) *fabric.Client {
	t.Helper()
	c, err := fabric.New(testConfig())
	if err != nil {
		t.Fatalf("connect fabric: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestRecordEventLandsOnChain submits an event and reads it back from the
// chaincode, asserting the ledger is authoritative.
func TestRecordEventLandsOnChain(t *testing.T) {
	requireIT(t)
	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	user := fmt.Sprintf("usr_it_%d", time.Now().UnixNano())
	written, err := c.RecordEvent(ctx, "goal", user, "goal_created", map[string]any{"name": "Europe Trip"})
	if err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	if written.TxID == "" {
		t.Error("expected a Fabric transaction id")
	}
	if written.Seq != 1 || written.PrevHash != "" {
		t.Errorf("first event should be seq 1 with empty prevHash, got seq=%d prev=%q", written.Seq, written.PrevHash)
	}
	if !written.SmartContract.Passed {
		t.Errorf("expected smart contract to pass, got violation %q", written.SmartContract.Violation)
	}

	onChain, err := c.QueryByUser(ctx, user)
	if err != nil {
		t.Fatalf("QueryByUser: %v", err)
	}
	if len(onChain) != 1 {
		t.Fatalf("expected 1 on-chain event, got %d", len(onChain))
	}
	if onChain[0].Hash != written.Hash {
		t.Errorf("on-chain hash %q != submitted hash %q", onChain[0].Hash, written.Hash)
	}

	byCat, err := c.QueryByCategory(ctx, user, "goal")
	if err != nil {
		t.Fatalf("QueryByCategory: %v", err)
	}
	if len(byCat) != 1 {
		t.Errorf("expected 1 event in category goal, got %d", len(byCat))
	}
}

// TestHashChainAndProof asserts events link into a verifiable per-user chain.
func TestHashChainAndProof(t *testing.T) {
	requireIT(t)
	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	user := fmt.Sprintf("usr_it_chain_%d", time.Now().UnixNano())
	var prev string
	for i := 1; i <= 3; i++ {
		ev, err := c.RecordEvent(ctx, "audit", user, "audit_entry", map[string]any{"n": i})
		if err != nil {
			t.Fatalf("RecordEvent %d: %v", i, err)
		}
		if ev.PrevHash != prev {
			t.Errorf("event %d prevHash = %q, want %q", i, ev.PrevHash, prev)
		}
		prev = ev.Hash
	}

	proof, err := c.GetProof(ctx, user)
	if err != nil {
		t.Fatalf("GetProof: %v", err)
	}
	if !proof.Intact {
		t.Errorf("chain reported broken: %s", proof.Detail)
	}
	if proof.EventCount != 3 {
		t.Errorf("EventCount = %d, want 3", proof.EventCount)
	}
	if proof.HeadHash != prev {
		t.Errorf("HeadHash = %q, want %q", proof.HeadHash, prev)
	}
	if proof.MerkleRoot == "" {
		t.Error("expected a Merkle root")
	}
}

// TestCoolingOffIsEnforcedOnChain is the guarantee the application layer cannot
// bypass: after a blocked transaction, a new one is rejected by the chaincode.
func TestCoolingOffIsEnforcedOnChain(t *testing.T) {
	requireIT(t)
	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	user := fmt.Sprintf("usr_it_cool_%d", time.Now().UnixNano())
	if _, err := c.RecordEvent(ctx, "transaction", user, "transaction_blocked",
		map[string]any{"reason": "high risk"}); err != nil {
		t.Fatalf("record blocked txn: %v", err)
	}

	_, err := c.RecordEvent(ctx, "transaction", user, "transaction_initiated",
		map[string]any{"amountPaise": 1000})
	if err == nil {
		t.Fatal("expected the cooling-off smart contract to REJECT the transaction")
	}

	active, remaining, err := c.VerifyCoolingOff(ctx, user)
	if err != nil {
		t.Fatalf("VerifyCoolingOff: %v", err)
	}
	if !active {
		t.Error("expected cooling-off to be active")
	}
	if remaining <= 0 || remaining > 6*time.Hour {
		t.Errorf("remaining = %v, want between 0 and 6h", remaining)
	}
}

// TestDoubleSpendIsEnforcedOnChain asserts an identical payload is rejected.
func TestDoubleSpendIsEnforcedOnChain(t *testing.T) {
	requireIT(t)
	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	user := fmt.Sprintf("usr_it_ds_%d", time.Now().UnixNano())
	payload := map[string]any{"amountPaise": 4242, "ref": "same-payload"}

	if _, err := c.RecordEvent(ctx, "transaction", user, "transaction_initiated", payload); err != nil {
		t.Fatalf("first transaction should succeed: %v", err)
	}
	if _, err := c.RecordEvent(ctx, "transaction", user, "transaction_initiated", payload); err == nil {
		t.Fatal("expected the double-spend smart contract to REJECT the replay")
	}

	events, err := c.QueryByUser(ctx, user)
	if err != nil {
		t.Fatalf("QueryByUser: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("rejected replay must not be committed: got %d events, want 1", len(events))
	}
}

// TestUnknownCategoryRejected asserts the category whitelist is enforced.
func TestUnknownCategoryRejected(t *testing.T) {
	requireIT(t)
	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.RecordEvent(ctx, "not_a_category", "usr_it_bad", "x", map[string]any{})
	if err == nil {
		t.Fatal("expected an unknown category to be rejected")
	}
}

// TestPostgresMirrorMatchesFabric is the sync contract: an event submitted via
// the client must appear in the Postgres read model with the same tx id/hash.
func TestPostgresMirrorMatchesFabric(t *testing.T) {
	requireIT(t)
	dsn := envOr("FABRIC_IT_DSN", "postgres://finix:finix@localhost:5432/finix?sslmode=disable")
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	c := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Mirror in-process rather than relying on the server's listener.
	syncer := fabric.NewSyncer(c, pool, nil)
	syncCtx, stopSync := context.WithCancel(ctx)
	defer stopSync()
	go syncer.Run(syncCtx)
	time.Sleep(2 * time.Second) // let the subscription establish

	user := fmt.Sprintf("usr_it_sync_%d", time.Now().UnixNano())
	written, err := c.RecordEvent(ctx, "payment", user, "payment_initiated",
		map[string]any{"amountPaise": 150000})
	if err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	// The mirror is asynchronous; poll briefly.
	var txID, hash string
	var blockNum int64
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		err = pool.QueryRow(ctx,
			`SELECT fabric_tx_id, hash, block_num FROM ledger_events WHERE user_id = $1 AND seq = $2`,
			user, int64(written.Seq)).Scan(&txID, &hash, &blockNum)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("event never mirrored to postgres: %v", err)
	}
	if txID != written.TxID {
		t.Errorf("postgres fabric_tx_id %q != fabric tx_id %q", txID, written.TxID)
	}
	if hash != written.Hash {
		t.Errorf("postgres hash %q != fabric hash %q", hash, written.Hash)
	}
	if blockNum <= 0 {
		t.Errorf("expected a committed block_num, got %d", blockNum)
	}
}

// TestLedgerAdapterSatisfiesDomain keeps the domain wiring honest without a network.
func TestLedgerAdapterSatisfiesDomain(t *testing.T) {
	var _ blockchain.FabricBackend = (*fabric.LedgerAdapter)(nil)
}
