package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroupFromPath(t *testing.T) {
	tests := []struct {
		path  string
		group string
	}{
		{path: "/healthz", group: "system"},
		{path: "/v1/auth/register", group: "auth"},
		{path: "/v1/transactions/initiate", group: "transactions"},
		{path: "/v1/goals", group: "goals"},
		{path: "/v1/portfolio/summary", group: "portfolio"},
		{path: "/unknown/path", group: "other"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			if got := groupFromPath(tt.path); got != tt.group {
				t.Fatalf("groupFromPath(%q) = %q, want %q", tt.path, got, tt.group)
			}
		})
	}
}

func TestFlowTrackerMiddlewareCapturesRequest(t *testing.T) {
	tracker := newFlowTracker(20)
	handler := tracker.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	snap := tracker.Snapshot()
	if snap.Totals.Requests != 1 {
		t.Fatalf("requests = %d, want 1", snap.Totals.Requests)
	}
	if len(snap.Recent) != 1 {
		t.Fatalf("recent size = %d, want 1", len(snap.Recent))
	}
	if snap.Recent[0].Group != "auth" {
		t.Fatalf("group = %s, want auth", snap.Recent[0].Group)
	}
}

func TestFlowTrackerRecentLimit(t *testing.T) {
	tracker := newFlowTracker(3)
	handler := tracker.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/system/version", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	snap := tracker.Snapshot()
	if len(snap.Recent) != 3 {
		t.Fatalf("recent size = %d, want 3", len(snap.Recent))
	}
}
