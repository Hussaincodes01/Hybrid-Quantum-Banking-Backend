package fabric

import (
	"context"
	"time"

	"FINIX/backend/internal/domain/blockchain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LedgerAdapter makes a *Client satisfy blockchain.FabricBackend.
//
// The domain package defines the interface in its own types so it stays free of
// infrastructure imports; this adapter is the only place the two shapes meet.
//
// Read routing (A5): when a Postgres pool is attached, QueryByUser/QueryByCategory
// serve from the off-chain ledger_events mirror (fast, no chaincode round-trip);
// writes and integrity proofs always go to Fabric (authoritative). If a Postgres
// read fails, the adapter falls back to the on-chain query so a mirror outage
// never blinds the API.
type LedgerAdapter struct {
	client *Client
	pool   *pgxpool.Pool
}

// NewLedgerAdapter wraps a connected client. Pass a non-nil pool to serve reads
// from the Postgres mirror; pass nil to read directly from Fabric.
func NewLedgerAdapter(c *Client, pool *pgxpool.Pool) *LedgerAdapter {
	return &LedgerAdapter{client: c, pool: pool}
}

// Compile-time proof the adapter satisfies the domain interface.
var _ blockchain.FabricBackend = (*LedgerAdapter)(nil)

func (a *LedgerAdapter) RecordEvent(ctx context.Context, category, userID, action string, payload any) (blockchain.FabricEvent, error) {
	ev, err := a.client.RecordEvent(ctx, category, userID, action, payload)
	if err != nil {
		return blockchain.FabricEvent{}, err
	}
	return toDomain(ev), nil
}

func (a *LedgerAdapter) QueryByUser(ctx context.Context, userID string) ([]blockchain.FabricEvent, error) {
	// Fast path: the off-chain mirror.
	if a.pool != nil {
		if events, err := a.readMirror(ctx, userID, ""); err == nil {
			return events, nil
		}
		// fall through to on-chain if the mirror is briefly unavailable
	}
	events, err := a.client.QueryByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toDomainSlice(events), nil
}

func (a *LedgerAdapter) QueryByCategory(ctx context.Context, userID, category string) ([]blockchain.FabricEvent, error) {
	if a.pool != nil {
		if events, err := a.readMirror(ctx, userID, category); err == nil {
			return events, nil
		}
	}
	events, err := a.client.QueryByCategory(ctx, userID, category)
	if err != nil {
		return nil, err
	}
	return toDomainSlice(events), nil
}

func (a *LedgerAdapter) GetProof(ctx context.Context, userID string) (blockchain.FabricProof, error) {
	p, err := a.client.GetProof(ctx, userID)
	if err != nil {
		return blockchain.FabricProof{}, err
	}
	return blockchain.FabricProof{
		MerkleRoot: p.MerkleRoot,
		EventCount: p.EventCount,
		HeadHash:   p.HeadHash,
		Intact:     p.Intact,
		Detail:     p.Detail,
	}, nil
}

func (a *LedgerAdapter) VerifyCoolingOff(ctx context.Context, userID string) (bool, time.Duration, error) {
	return a.client.VerifyCoolingOff(ctx, userID)
}

func (a *LedgerAdapter) BlockHeight(ctx context.Context) (uint64, error) {
	return a.client.BlockHeight(ctx)
}

// readMirror serves a user's events from the Postgres ledger_events table,
// oldest first (matching the chaincode's ordering). category == "" returns all.
func (a *LedgerAdapter) readMirror(ctx context.Context, userID, category string) ([]blockchain.FabricEvent, error) {
	const cols = `event_id, user_id, action, resource, category,
		payload_hash, prev_hash, hash, event_time, seq, fabric_tx_id,
		rule_name, rule_passed, rule_violation`

	var (
		rows pgx.Rows
		err  error
	)
	if category == "" {
		rows, err = a.pool.Query(ctx,
			`SELECT `+cols+` FROM ledger_events WHERE user_id = $1 ORDER BY seq ASC`, userID)
	} else {
		rows, err = a.pool.Query(ctx,
			`SELECT `+cols+` FROM ledger_events WHERE user_id = $1 AND category = $2 ORDER BY seq ASC`,
			userID, category)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]blockchain.FabricEvent, 0, 16)
	for rows.Next() {
		var (
			fe        blockchain.FabricEvent
			seq       int64
			eventTime time.Time
			sc        blockchain.SmartContractResult
		)
		if err := rows.Scan(
			&fe.ID, &fe.UserID, &fe.Action, &fe.Resource, &fe.Category,
			&fe.PayloadHash, &fe.PrevHash, &fe.Hash, &eventTime, &seq, &fe.TxID,
			&sc.RuleName, &sc.Passed, &sc.Violation,
		); err != nil {
			return nil, err
		}
		fe.Timestamp = eventTime
		fe.Seq = uint64(seq)
		fe.SmartContract = sc
		out = append(out, fe)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func toDomain(ev Event) blockchain.FabricEvent {
	return blockchain.FabricEvent{
		ID:          ev.ID,
		UserID:      ev.UserID,
		Action:      ev.Action,
		Resource:    ev.Resource,
		Category:    ev.Category,
		PayloadHash: ev.PayloadHash,
		PrevHash:    ev.PrevHash,
		Hash:        ev.Hash,
		Timestamp:   ev.Timestamp,
		Seq:         ev.Seq,
		TxID:        ev.TxID,
		SmartContract: blockchain.SmartContractResult{
			RuleName:   ev.SmartContract.RuleName,
			Passed:     ev.SmartContract.Passed,
			Violation:  ev.SmartContract.Violation,
			EnforcedAt: ev.SmartContract.EnforcedAt,
		},
	}
}

func toDomainSlice(events []Event) []blockchain.FabricEvent {
	out := make([]blockchain.FabricEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, toDomain(ev))
	}
	return out
}
