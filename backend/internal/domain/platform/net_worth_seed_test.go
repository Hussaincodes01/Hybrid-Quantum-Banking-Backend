package platform

import "testing"

// TestSeededNetWorthIsPositive verifies that /v1/portfolio/net-worth (backed by
// NetWorthSnapshot) reports a positive net worth for the demo user Jiyad, whose
// large home loan previously outweighed the seeded balance. The seed top-up now
// mirrors NetWorthSnapshot's asset definition exactly, so the reported net worth
// equals the intended NetWorthLakhs (+₹85,00,000).
func TestSeededNetWorthIsPositive(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	if svc.NeedsSeeding() {
		if err := svc.SeedDemoUsers(); err != nil {
			t.Fatalf("seed demo users: %v", err)
		}
	}

	var jiyadID string
	for id, u := range svc.users {
		if u.Name == "Jiyad" {
			jiyadID = id
			break
		}
	}
	if jiyadID == "" {
		t.Fatal("seeded user Jiyad not found")
	}

	snap, err := svc.NetWorthSnapshot(jiyadID)
	if err != nil {
		t.Fatalf("NetWorthSnapshot: %v", err)
	}
	if snap.NetWorthPaise <= 0 {
		t.Fatalf("net worth must be positive, got %d paise (assets %d, liabilities %d)",
			snap.NetWorthPaise, snap.TotalAssetsPaise, snap.TotalLiabilitiesPaise)
	}

	const wantPaise = int64(85) * 100000 * 100 // ₹85,00,000
	if snap.NetWorthPaise != wantPaise {
		t.Errorf("net worth = %d paise, want %d (+₹85 Lakhs)", snap.NetWorthPaise, wantPaise)
	}
}
