package healthscore

import (
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

// monthsAgo returns a timestamp inside the calendar month n months back, which
// is what monthlyBuckets keys on.
func monthsAgo(n int) time.Time { return now.AddDate(0, -n, 0) }

// steadyEarner spends 60k of a 100k monthly income for six months.
func steadyEarner() []TxnFact {
	var txns []TxnFact
	for m := 0; m < 6; m++ {
		txns = append(txns,
			TxnFact{AmountPaise: 10_000_000, Credit: true, At: monthsAgo(m)},
			TxnFact{AmountPaise: 6_000_000, Credit: false, At: monthsAgo(m)},
		)
	}
	return txns
}

func TestDerive_ScoreDiffersBetweenCustomers(t *testing.T) {
	// The bug this whole file exists for: two very different customers used to
	// score within a point of each other because six of seven inputs were
	// constants.
	healthy := Derive(Facts{
		Now:                 now,
		LiquidBalancePaise:  60_000_000, // 6 months of spend
		Transactions:        steadyEarner(),
		InsuranceCoverPaise: 150_000_000,
		Investments: []HoldingFact{
			{Category: "equity", CurrentValuePaise: 25_000_000},
			{Category: "debt", CurrentValuePaise: 25_000_000},
			{Category: "gold", CurrentValuePaise: 25_000_000},
			{Category: "cash", CurrentValuePaise: 25_000_000},
		},
		Goals: []GoalFact{{
			SavedPaise: 90, TargetPaise: 100, Priority: "high",
			StartDate: monthsAgo(10), TargetDate: now.AddDate(0, 2, 0),
		}},
	})

	stretched := Derive(Facts{
		Now:                now,
		LiquidBalancePaise: 2_000_000, // under a month of cover
		Transactions: func() []TxnFact {
			var txns []TxnFact
			for m := 0; m < 6; m++ {
				txns = append(txns,
					TxnFact{AmountPaise: 10_000_000, Credit: true, At: monthsAgo(m)},
					TxnFact{AmountPaise: 9_800_000, Credit: false, At: monthsAgo(m)},
				)
			}
			return txns
		}(),
		LiabilitiesPaise: 500_000_000,
		MonthlyEMIPaise:  6_000_000,
		Investments:      []HoldingFact{{Category: "equity", CurrentValuePaise: 1_000_000}},
		Goals: []GoalFact{{
			SavedPaise: 5, TargetPaise: 100, Priority: "high",
			StartDate: monthsAgo(10), TargetDate: now.AddDate(0, 2, 0),
		}},
	})

	calc := NewCalculator()
	healthyScore := calc.Calculate(healthy).Score300To900
	stretchedScore := calc.Calculate(stretched).Score300To900

	if healthyScore <= stretchedScore {
		t.Fatalf("a well-provisioned customer must outscore a stretched one: healthy=%d stretched=%d",
			healthyScore, stretchedScore)
	}
	if healthyScore-stretchedScore < 100 {
		t.Errorf("scores should separate meaningfully, got healthy=%d stretched=%d (gap %d)",
			healthyScore, stretchedScore, healthyScore-stretchedScore)
	}
}

func TestDerive_EmptyCustomerIsSafe(t *testing.T) {
	// A brand-new account has no transactions at all. This must not divide by
	// zero, produce NaN, or panic.
	in := Derive(Facts{Now: now})
	res := NewCalculator().Calculate(in)

	if math.IsNaN(float64(res.Score300To900)) {
		t.Fatal("score is NaN")
	}
	if res.Score300To900 < 300 || res.Score300To900 > 900 {
		t.Fatalf("score out of range: %d", res.Score300To900)
	}
	for _, p := range res.Pillars {
		if math.IsNaN(p.Score) || p.Score < 0 || p.Score > 100 {
			t.Errorf("pillar %s out of range: %v", p.Name, p.Score)
		}
	}
}

func TestEmergencyFundMonths(t *testing.T) {
	if got := emergencyFundMonths(60_000_000, 10_000_000); got != 6 {
		t.Errorf("6 months of cover expected, got %v", got)
	}
	// No spending history: cash on hand should not read as infinite cover.
	if got := emergencyFundMonths(50_000, 0); got != 6 {
		t.Errorf("no-spend case should report the 6-month target, got %v", got)
	}
	if got := emergencyFundMonths(0, 0); got != 0 {
		t.Errorf("no cash and no spend should be 0, got %v", got)
	}
}

func TestSavingsRate_UnobservedIncomeIsNeutralNotZero(t *testing.T) {
	// Seed accounts carry spending but no salary credits. Reporting 0 there
	// would score the pillar at zero, asserting the customer saves nothing,
	// when in fact their income simply is not visible to us.
	unobserved := savingsRate(0, 500)
	if unobserved != neutralSavingsRate {
		t.Fatalf("no observed income should be neutral, got %v", unobserved)
	}
	score := ratioScore(unobserved, savingsRateTarget)
	if score < 55 || score > 65 {
		t.Errorf("neutral savings should land near 60/100, got %v", score)
	}

	// Someone who genuinely spends everything they earn still scores 0.
	if spentAll := savingsRate(1000, 1000); spentAll != 0 {
		t.Errorf("spending all income is a real 0, got %v", spentAll)
	}
}

func TestDebtToIncome_UsesIncomeNotAssets(t *testing.T) {
	// 60k EMI against 200k income is 30%.
	got := debtToIncome(6_000_000, 20_000_000, 0, 0)
	if math.Abs(got-0.3) > 1e-9 {
		t.Errorf("expected 0.30, got %v", got)
	}
	// With no observed income it falls back to leverage rather than claiming
	// the customer is debt-free.
	if got := debtToIncome(0, 0, 100, 100); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("leverage fallback expected 0.5, got %v", got)
	}
}

func TestDiversification_ConcentrationIsPenalised(t *testing.T) {
	spread := diversification([]HoldingFact{
		{Category: "equity", CurrentValuePaise: 100},
		{Category: "debt", CurrentValuePaise: 100},
		{Category: "gold", CurrentValuePaise: 100},
		{Category: "cash", CurrentValuePaise: 100},
	})
	// Same number of holdings, but all one asset class.
	concentrated := diversification([]HoldingFact{
		{Category: "equity", CurrentValuePaise: 100},
		{Category: "equity", CurrentValuePaise: 100},
		{Category: "equity", CurrentValuePaise: 100},
		{Category: "equity", CurrentValuePaise: 100},
	})
	if spread <= concentrated {
		t.Errorf("four asset classes must beat four equity funds: spread=%v concentrated=%v",
			spread, concentrated)
	}
	if none := diversification(nil); none != 0 {
		t.Errorf("no holdings should score 0, got %v", none)
	}
}

func TestGoalOnTrack_ComparesAgainstElapsedTime(t *testing.T) {
	// Halfway through the timeline with half the money saved is on track.
	onTrack := goalOnTrack([]GoalFact{{
		SavedPaise: 50, TargetPaise: 100,
		StartDate: now.AddDate(0, -6, 0), TargetDate: now.AddDate(0, 6, 0),
	}}, now)
	if onTrack < 0.95 {
		t.Errorf("saving in line with elapsed time should be on track, got %v", onTrack)
	}

	// Same elapsed time, barely any progress.
	behind := goalOnTrack([]GoalFact{{
		SavedPaise: 5, TargetPaise: 100,
		StartDate: now.AddDate(0, -6, 0), TargetDate: now.AddDate(0, 6, 0),
	}}, now)
	if behind >= onTrack {
		t.Errorf("a lagging goal must score below an on-track one: behind=%v onTrack=%v", behind, onTrack)
	}
	if got := goalOnTrack(nil, now); got != 0 {
		t.Errorf("no goals should be 0, got %v", got)
	}
}

func TestBehaviourQuality_RewardsConsistency(t *testing.T) {
	steady := behaviourQuality(monthlyBuckets(steadyEarner(), now))

	var erratic []TxnFact
	swings := []int64{1_000_000, 14_000_000, 2_000_000, 18_000_000, 500_000, 20_000_000}
	for m, amt := range swings {
		erratic = append(erratic,
			TxnFact{AmountPaise: 10_000_000, Credit: true, At: monthsAgo(m)},
			TxnFact{AmountPaise: amt, Credit: false, At: monthsAgo(m)},
		)
	}
	volatile := behaviourQuality(monthlyBuckets(erratic, now))

	if steady <= volatile {
		t.Errorf("steady spending must outscore erratic: steady=%v volatile=%v", steady, volatile)
	}
}

func TestMonthlyBuckets_IgnoresFutureAndStaleTransactions(t *testing.T) {
	txns := []TxnFact{
		{AmountPaise: 100, Credit: true, At: now.AddDate(0, 1, 0)},  // future
		{AmountPaise: 100, Credit: true, At: now.AddDate(-2, 0, 0)}, // two years old
		{AmountPaise: 100, Credit: true, At: now},                   // current month
	}
	buckets := monthlyBuckets(txns, now)
	observed := 0
	for _, b := range buckets {
		if b.hasObservation {
			observed++
		}
	}
	if observed != 1 {
		t.Errorf("only the in-window transaction should count, got %d months with activity", observed)
	}
}

func TestDerive_ScoreRespondsToNewSpending(t *testing.T) {
	// The score has to move when the customer acts — that was the whole
	// complaint about it being static.
	base := Facts{
		Now:                now,
		LiquidBalancePaise: 60_000_000,
		Transactions:       steadyEarner(),
	}
	before := NewCalculator().Calculate(Derive(base)).Score300To900

	// A month where they spent nearly everything they earned.
	worse := base
	worse.Transactions = append(append([]TxnFact{}, base.Transactions...),
		TxnFact{AmountPaise: 9_500_000, Credit: false, At: now})
	after := NewCalculator().Calculate(Derive(worse)).Score300To900

	if after >= before {
		t.Errorf("spending more should lower the score: before=%d after=%d", before, after)
	}
}
