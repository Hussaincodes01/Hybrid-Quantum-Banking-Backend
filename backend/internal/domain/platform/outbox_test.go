package platform

import (
	"testing"
	"time"
)

// M-12: a failed publish must retry (up to 3) then dead-letter, never silently drop.
func TestOutboxMarkFailedRetriesThenDeadLetters(t *testing.T) {
	o := NewOutbox()
	o.Enqueue("txn", "payload")

	ev := o.Dequeue(1)
	if len(ev) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(ev))
	}
	id := ev[0].ID

	// Two failures keep it pending for retry.
	o.MarkFailed(id)
	o.MarkFailed(id)
	if o.PendingCount() != 1 {
		t.Fatalf("event should still be pending after 2 failures, pending=%d", o.PendingCount())
	}
	// Third failure dead-letters it (Processed=true so it is skipped, not retried forever).
	o.MarkFailed(id)
	if o.PendingCount() != 0 {
		t.Fatalf("event should be dead-lettered after 3 failures, pending=%d", o.PendingCount())
	}
}

// M-12: Purge drops long-processed events so the slice can't grow unbounded, while
// keeping unprocessed ones for retry.
func TestOutboxPurgeKeepsUnprocessed(t *testing.T) {
	o := NewOutbox()
	o.Enqueue("a", "1")
	o.Enqueue("b", "2")

	pending := o.Dequeue(10)
	// Process the first, leave the second pending.
	o.MarkProcessed(pending[0].ID)

	// Backdate every event so the processed one is eligible for purge.
	o.mu.Lock()
	for _, e := range o.events {
		e.CreatedAt = time.Now().UTC().Add(-24 * time.Hour)
	}
	o.mu.Unlock()

	o.Purge(6 * time.Hour)

	o.mu.Lock()
	remaining := len(o.events)
	o.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("purge should keep only the 1 unprocessed event, got %d", remaining)
	}
	if o.PendingCount() != 1 {
		t.Fatalf("unprocessed event must survive purge, pending=%d", o.PendingCount())
	}
}

// H-10: the idempotency cache honours its TTL and hard cap.
func TestIdempotencyCacheEviction(t *testing.T) {
	s := NewService()

	s.mu.Lock()
	// Expired entry is pruned.
	s.idempotencyResult["u:expired"] = TransactionResult{Status: "old"}
	s.idempotencyExpiry["u:expired"] = time.Now().UTC().Add(-time.Minute)
	// Fresh entry survives.
	s.setIdempotentLocked("u:fresh", TransactionResult{Status: "new"})
	s.pruneIdempotencyLocked()
	_, expiredPresent := s.idempotencyResult["u:expired"]
	_, freshPresent := s.idempotencyResult["u:fresh"]
	s.mu.Unlock()

	if expiredPresent {
		t.Fatal("expired idempotency entry should have been evicted")
	}
	if !freshPresent {
		t.Fatal("fresh idempotency entry should survive")
	}
}
