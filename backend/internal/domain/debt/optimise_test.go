package debt

import (
	"testing"

	"FINIX/backend/internal/config"
)

func TestPrepaymentReducesTenureNotEMI(t *testing.T) {
	loan := Loan{PrincipalPaise: 100_000_000, AnnualRatePct: 9.0, Months: 60}
	res := PrepaymentImpact(loan, 20_000_000, 12) // ₹2,00,000 extra at month 12
	if res.TenureReducedMonths <= 0 {
		t.Fatalf("expected tenure reduction, got %d", res.TenureReducedMonths)
	}
	if res.InterestSavedPaise <= 0 {
		t.Fatalf("expected positive interest saved, got %d", res.InterestSavedPaise)
	}
	if res.NewMonths >= res.BaselineMonths {
		t.Fatalf("prepayment should shorten tenure: baseline=%d new=%d", res.BaselineMonths, res.NewMonths)
	}
}

func TestForeclosurePenaltyFloatingRetailZero(t *testing.T) {
	p := config.Default().Foreclosure
	// Floating individual retail loan → 0 penalty (RBI 2014).
	if rate := ForeclosurePenaltyRate("home", true, p); rate != 0 {
		t.Fatalf("floating home loan penalty = %v, want 0", rate)
	}
	if rate := ForeclosurePenaltyRate("personal", true, p); rate != 0 {
		t.Fatalf("floating personal loan penalty = %v, want 0", rate)
	}
	// Fixed-rate retail loan → penalty applies.
	if rate := ForeclosurePenaltyRate("home", false, p); rate <= 0 {
		t.Fatalf("fixed home loan penalty = %v, want > 0", rate)
	}
	// Business loan (even floating) → penalty applies.
	if rate := ForeclosurePenaltyRate("business", true, p); rate <= 0 {
		t.Fatalf("business loan penalty = %v, want > 0", rate)
	}
}

func TestForeclosureNegativeWhenLoanCheaperThanReturns(t *testing.T) {
	p := config.Default().Foreclosure // expected return 11%
	// A cheap floating home loan (7%) with zero penalty: cash used to foreclose
	// would earn more invested, so NetBenefit should be negative.
	loan := Loan{PrincipalPaise: 100_000_000, AnnualRatePct: 7.0, Months: 120, Floating: true, LoanType: "home"}
	res := ForeclosureDecision(loan, loan.PrincipalPaise, p)
	if res.PenaltyAmountPaise != 0 {
		t.Fatalf("floating retail penalty should be 0, got %d", res.PenaltyAmountPaise)
	}
	if res.OpportunityCostPaise <= 0 {
		t.Fatalf("expected positive opportunity cost, got %d", res.OpportunityCostPaise)
	}
	if res.NetBenefitPaise >= 0 || res.Recommended {
		t.Fatalf("cheap loan below expected return should NOT be recommended: net=%d rec=%v", res.NetBenefitPaise, res.Recommended)
	}
}

func TestSnowballNeverBeatsAvalanche(t *testing.T) {
	p := config.Default().Debt
	loans := []Loan{
		{ID: "a", PrincipalPaise: 5_000_000, AnnualRatePct: 22.0, Months: 24}, // small, high rate
		{ID: "b", PrincipalPaise: 30_000_000, AnnualRatePct: 9.0, Months: 60}, // large, low rate
		{ID: "c", PrincipalPaise: 1_000_000, AnnualRatePct: 12.0, Months: 12}, // tiny, mid rate
	}
	res := OptimiseStrategy(loans, 2_000_000, 0.8, p)
	if res.SnowballInterestPaise < res.AvalancheInterestPaise {
		t.Fatalf("snowball (%d) must never be cheaper than avalanche (%d)", res.SnowballInterestPaise, res.AvalancheInterestPaise)
	}
	if res.InterestCostOfSnowball < 0 {
		t.Fatalf("interest cost of snowball must be >= 0, got %d", res.InterestCostOfSnowball)
	}
	// High adherence → avalanche recommended.
	if res.RecommendedStrategy != StrategyAvalanche {
		t.Fatalf("high adherence should recommend avalanche, got %s", res.RecommendedStrategy)
	}
}

func TestLowAdherenceRecommendsSnowball(t *testing.T) {
	p := config.Default().Debt
	loans := []Loan{
		{ID: "a", PrincipalPaise: 5_000_000, AnnualRatePct: 22.0, Months: 24},
		{ID: "b", PrincipalPaise: 30_000_000, AnnualRatePct: 9.0, Months: 60},
	}
	res := OptimiseStrategy(loans, 2_000_000, 0.3, p) // documented low adherence
	if res.RecommendedStrategy != StrategySnowball {
		t.Fatalf("low adherence should recommend snowball, got %s", res.RecommendedStrategy)
	}
	// Both numbers must still be present.
	if res.AvalancheInterestPaise <= 0 || res.SnowballInterestPaise <= 0 {
		t.Fatal("both waterfall totals must be reported regardless of recommendation")
	}
}

func TestRepoRateRecompute(t *testing.T) {
	loan := Loan{PrincipalPaise: 320_000_000, AnnualRatePct: 8.55, Months: 212, EMIPaise: 2_850_000, Floating: true, LoanType: "home"}
	// +50 bps with full transmission → +0.50 pct points.
	res := RepoRateRecompute(loan, 50, 1.0)
	if res.NewRatePct <= res.OldRatePct {
		t.Fatalf("rate should rise: old=%.2f new=%.2f", res.OldRatePct, res.NewRatePct)
	}
	if res.NewEMIPaise <= res.OldEMIPaise {
		t.Fatalf("EMI should rise with the rate: old=%d new=%d", res.OldEMIPaise, res.NewEMIPaise)
	}
	if res.NewRatePct < 9.04 || res.NewRatePct > 9.06 {
		t.Fatalf("new rate = %.2f, want ≈ 9.05", res.NewRatePct)
	}
}
