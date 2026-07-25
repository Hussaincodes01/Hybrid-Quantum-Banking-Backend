package fabric

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Syncer mirrors committed Fabric chaincode events into Postgres.
//
// Fabric stays the source of truth; Postgres is a derived read model so the
// HTTP API can answer dashboard / audit / cooling-off queries with SQL instead
// of a chaincode round-trip. The listener is resumable: it records the last
// mirrored block so a restart continues rather than replaying the chain.
type Syncer struct {
	client *Client
	pool   *pgxpool.Pool
	log    *slog.Logger
}

// NewSyncer wires a connected Fabric client to a Postgres pool.
func NewSyncer(c *Client, pool *pgxpool.Pool, log *slog.Logger) *Syncer {
	if log == nil {
		log = slog.Default()
	}
	return &Syncer{client: c, pool: pool, log: log}
}

// Run streams chaincode events into Postgres until ctx is cancelled. It is
// resilient: a dropped stream is retried with backoff rather than killing the
// process, because losing the mirror must never take the API down.
func (s *Syncer) Run(ctx context.Context) {
	if s.client == nil || s.pool == nil {
		s.log.Warn("ledger sync disabled: fabric client or postgres pool missing")
		return
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.stream(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("ledger sync stream ended; retrying", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		return
	}
}

// stream subscribes from the last mirrored block and writes each event.
func (s *Syncer) stream(ctx context.Context) error {
	start, err := s.lastBlock(ctx)
	if err != nil {
		return fmt.Errorf("read sync state: %w", err)
	}

	events, err := s.client.Events(ctx, start)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.log.Info("ledger sync started", "from_block", start)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				return fmt.Errorf("event stream closed")
			}
			if err := s.upsert(ctx, ev); err != nil {
				// Log and continue: Fabric still holds the authoritative record,
				// and a later replay can re-mirror this event.
				s.log.Error("mirror event failed", "tx_id", ev.TxID, "user_id", ev.UserID, "error", err)
				continue
			}
			// Advance the resume watermark so a restart continues from the next
			// block instead of replaying the whole chain.
			if ev.BlockNum > 0 {
				if err := s.SetLastBlock(ctx, ev.BlockNum); err != nil {
					s.log.Warn("advance sync watermark failed", "block", ev.BlockNum, "error", err)
				}
			}
			s.log.Debug("mirrored ledger event",
				"category", ev.Category, "action", ev.Action, "user_id", ev.UserID,
				"seq", ev.Seq, "block", ev.BlockNum)
		}
	}
}

// upsert writes one event into the mirror. Idempotent on (user_id, seq) so a
// replayed block cannot duplicate rows.
func (s *Syncer) upsert(ctx context.Context, ev Event) error {
	payload := ev.Payload
	if payload == "" || !json.Valid([]byte(payload)) {
		payload = "{}"
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO ledger_events (
			event_id, user_id, category, action, resource,
			payload_json, payload_hash, prev_hash, hash,
			seq, fabric_tx_id, block_num, rule_name, rule_passed, rule_violation, event_time
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (user_id, seq) DO UPDATE SET
			fabric_tx_id = EXCLUDED.fabric_tx_id,
			block_num    = EXCLUDED.block_num,
			hash         = EXCLUDED.hash`,
		ev.ID, ev.UserID, ev.Category, ev.Action, ev.Resource,
		payload, ev.PayloadHash, ev.PrevHash, ev.Hash,
		int64(ev.Seq), ev.TxID, int64(ev.BlockNum),
		ev.SmartContract.RuleName, ev.SmartContract.Passed, ev.SmartContract.Violation,
		ev.Timestamp,
	)
	return err
}

// lastBlock returns the block to resume from.
func (s *Syncer) lastBlock(ctx context.Context) (uint64, error) {
	var last int64
	err := s.pool.QueryRow(ctx,
		`SELECT last_block_num FROM ledger_sync_state WHERE id = 1`).Scan(&last)
	if err != nil {
		return 0, err
	}
	if last < 0 {
		return 0, nil
	}
	return uint64(last), nil
}

// SetLastBlock records mirroring progress.
func (s *Syncer) SetLastBlock(ctx context.Context, block uint64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ledger_sync_state SET last_block_num = $1, updated_at = NOW() WHERE id = 1`,
		int64(block))
	return err
}
