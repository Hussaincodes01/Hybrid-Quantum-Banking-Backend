package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ModelParams is the single, versioned source of truth for every threshold,
// weight, and rate used by the FINIX quantitative models (spec: "Srishti Math").
//
// The spec's thesis is model-risk auditability (RBI): no magic literals scattered
// through handlers — every number is a named, versioned field here so a change is
// a config bump with a traceable Version, not a silent code edit. Domain packages
// accept the sub-structs they need; the service loads ModelParams once at startup.
//
// Money is int64 paise throughout; rates are decimals (0.08 == 8%) unless a field
// name ends in Pct (percentage points, e.g. 9.0 == 9%).
type ModelParams struct {
	// Version is stamped into every audit artefact (health-score snapshot,
	// simulation Assumptions) so a score can be reproduced against the exact
	// parameter set that produced it. Bump on any field change.
	Version string `json:"version"`

	RiskTiers   RiskTierParams    `json:"riskTiers"`
	AmountTerm  AmountTermParams  `json:"amountTerm"`
	CoolingOff  CoolingOffParams  `json:"coolingOff"`
	Geo         GeoParams         `json:"geo"`
	Simulation  SimulationParams  `json:"simulation"`
	// ColdStartMonths gates ONNX inference: accounts younger than this fall back
	// to the heuristic (see transaction/risk_engine.go ColdStart, MODEL_IO_CONTRACT.md).
	ColdStartMonths int               `json:"coldStartMonths"`
	Foreclosure     ForeclosureParams `json:"foreclosure"`
	HealthScore     HealthScoreParams `json:"healthScore"`
	Structuring StructuringParams `json:"structuring"`
	Credit      CreditParams      `json:"credit"`
	Debt        DebtParams        `json:"debt"`
}

// RiskTierParams are the transaction risk-score cutoffs (§1.1). Score in [0,100].
type RiskTierParams struct {
	Medium float64 `json:"medium"` // score >= Medium -> MEDIUM tier
	High   float64 `json:"high"`   // score >= High   -> HIGH tier
}

// AmountTermParams shape the amount-anomaly term g(x)=clip((x-1)*K,0,Cap) (§1.1).
type AmountTermParams struct {
	K              float64 `json:"k"`              // slope per unit of (amountVsAverage-1)
	Cap            float64 `json:"cap"`            // max contribution of the amount term
	ClipMax        float64 `json:"clipMax"`        // hard ceiling on amountVsAverage before weighting
	FloorAvgPaise  int64   `json:"floorAvgPaise"`  // FLOOR_AVG denominator floor (₹500 = 50000 paise)
	BalanceClipMax float64 `json:"balanceClipMax"` // ceiling on balanceImpact (1.0)
}

// CoolingOffParams drive P_high-proportional cooling-off + friction (§1.4).
type CoolingOffParams struct {
	BaseMinutes int     `json:"baseMinutes"` // spec base = 240 min (4h)
	Kappa       float64 `json:"kappa"`       // proportional cooling-off slope
	Lambda      float64 `json:"lambda"`      // proportional override-friction slope
	PHighPivot  float64 `json:"pHighPivot"`  // reference P_high (0.70) above which escalation starts
}

// GeoParams drive the impossible-travel / geo term (§1.3).
type GeoParams struct {
	ImpossibleTravelKmh float64 `json:"impossibleTravelKmh"` // 900 km/h
	ImpossibleTerm      float64 `json:"impossibleTerm"`      // 25
	ForeignNoHistTerm   float64 `json:"foreignNoHistTerm"`   // 10
	DowZPivot           float64 `json:"dowZPivot"`           // 1.5
	DowTermCap          float64 `json:"dowTermCap"`          // 10
	DowMinObservations  int     `json:"dowMinObservations"`  // 4
	MarketAmountMult    float64 `json:"marketAmountMult"`    // 3 (amountVsAverage threshold)
	MarketVolPercentile float64 `json:"marketVolPercentile"` // 90
	MarketTerm          float64 `json:"marketTerm"`          // 8
}

// SimulationParams drive the Monte Carlo returns model (§5).
type SimulationParams struct {
	Mu              float64 `json:"mu"`              // 0.07 mean annual return
	Sigma           float64 `json:"sigma"`           // 0.12 annual stddev
	Blend           string  `json:"blend"`           // "normal" | "bootstrap"
	HistoricalYears int     `json:"historicalYears"` // window for bootstrap
	Count           int     `json:"count"`           // N paths
	Disclaimer      string  `json:"disclaimer"`      // spec S22.5 disclaimer
}

// ForeclosureParams drive loan foreclosure economics (§4.2).
type ForeclosureParams struct {
	// FixedRateLenderPenaltyRate applies to fixed-rate / business loans. Floating
	// individual retail loans are ALWAYS 0 per RBI 2014 (enforced in code).
	FixedRateLenderPenaltyRate float64 `json:"fixedRateLenderPenaltyRate"` // e.g. 0.02
	ExpectedInvestmentReturn   float64 `json:"expectedInvestmentReturn"`   // opportunity-cost benchmark, e.g. 0.11
	RepoTransmissionFactor     float64 `json:"repoTransmissionFactor"`     // 0..1 pass-through of repo delta
}

// HealthScoreParams drive retirement corpus + pillar math (§2).
type HealthScoreParams struct {
	RetirementAge      int     `json:"retirementAge"`      // 60
	ReplacementRate    float64 `json:"replacementRate"`    // 0.70
	PostYears          int     `json:"postYears"`          // 25 (annuity horizon)
	RealReturn         float64 `json:"realReturn"`         // 0.03 post-retirement real return
	Inflation          float64 `json:"inflation"`          // pre-retirement inflation
	CVFloor            float64 `json:"cvFloor"`            // small positive floor for coefficient-of-variation
	IdleCashFreeMonths int     `json:"idleCashFreeMonths"` // 3 months idle before penalty
}

// StructuringParams drive AML smurfing detection (§1.5).
type StructuringParams struct {
	BenchmarkPaise   int64   `json:"benchmarkPaise"`   // SingleTxnStructuringBenchmark
	MinCount         int     `json:"minCount"`         // 3 new recipients
	WindowHours      int     `json:"windowHours"`      // 24
	SumFraction      float64 `json:"sumFraction"`      // 0.9 of benchmark
	UniformityStddev float64 `json:"uniformityStddev"` // 0.15 * mean
}

// CreditParams drive utilisation + BNPL (§8).
type CreditParams struct {
	UtilisationAlert float64 `json:"utilisationAlert"` // 0.30
	AlertLeadDays    int     `json:"alertLeadDays"`    // T-5
	// BNPLProviders is the versioned registry of Buy-Now-Pay-Later counterparty
	// names (lower-cased, substring-matched). BNPLMerchantCategoryCodes lists MCCs
	// that indicate a BNPL settlement.
	BNPLProviders             []string `json:"bnplProviders"`
	BNPLMerchantCategoryCodes []string `json:"bnplMerchantCategoryCodes"`
}

// DebtParams drive optimisation strategy selection (§4.3).
type DebtParams struct {
	// LowAdherenceThreshold: below this documented adherence score, snowball's
	// behavioural benefit can justify its higher interest cost.
	LowAdherenceThreshold float64 `json:"lowAdherenceThreshold"` // 0.5
}

// Default returns the spec's canonical parameter set (v1).
func Default() ModelParams {
	return ModelParams{
		Version:   "srishti-math-v1",
		RiskTiers: RiskTierParams{Medium: 40, High: 70},
		AmountTerm: AmountTermParams{
			K: 12, Cap: 30, ClipMax: 25, FloorAvgPaise: 50000, BalanceClipMax: 1.0,
		},
		CoolingOff: CoolingOffParams{BaseMinutes: 240, Kappa: 2, Lambda: 1, PHighPivot: 0.70},
		Geo: GeoParams{
			ImpossibleTravelKmh: 900, ImpossibleTerm: 25, ForeignNoHistTerm: 10,
			DowZPivot: 1.5, DowTermCap: 10, DowMinObservations: 4,
			MarketAmountMult: 3, MarketVolPercentile: 90, MarketTerm: 8,
		},
		Simulation: SimulationParams{
			Mu: 0.07, Sigma: 0.12, Blend: "normal", HistoricalYears: 10, Count: 10000,
			Disclaimer: "Projections are based on historical patterns, not a prediction or guarantee of future returns.",
		},
		Foreclosure: ForeclosureParams{
			FixedRateLenderPenaltyRate: 0.02, ExpectedInvestmentReturn: 0.11, RepoTransmissionFactor: 1.0,
		},
		HealthScore: HealthScoreParams{
			RetirementAge: 60, ReplacementRate: 0.70, PostYears: 25, RealReturn: 0.03,
			Inflation: 0.05, CVFloor: 0.01, IdleCashFreeMonths: 3,
		},
		ColdStartMonths: 6,
		Structuring: StructuringParams{
			BenchmarkPaise: 100000000, MinCount: 3, WindowHours: 24, SumFraction: 0.9, UniformityStddev: 0.15,
		},
		Credit: CreditParams{
			UtilisationAlert: 0.30, AlertLeadDays: 5,
			BNPLProviders: []string{
				"lazypay", "simpl", "zestmoney", "slice", "amazon pay later",
				"flipkart pay later", "paytm postpaid", "ollo", "kreditbee", "mobikwik zip",
			},
			BNPLMerchantCategoryCodes: []string{"6012", "6051"},
		},
		Debt: DebtParams{LowAdherenceThreshold: 0.5},
	}
}

// LoadModelParams returns Default() with a small set of env overrides applied, so
// operators can retune the highest-leverage thresholds without a rebuild. Any
// override that fails to parse is ignored (the default stands). The Version is
// suffixed with "+override" whenever any override is applied, so audit artefacts
// never claim to be the pristine v1 set when they are not.
func LoadModelParams() ModelParams {
	p := Default()
	overridden := false

	if v, ok := modelEnvFloat("FINIX_RISK_TIER_MEDIUM"); ok {
		p.RiskTiers.Medium = v
		overridden = true
	}
	if v, ok := modelEnvFloat("FINIX_RISK_TIER_HIGH"); ok {
		p.RiskTiers.High = v
		overridden = true
	}
	if v, ok := modelEnvInt("FINIX_COOLOFF_BASE_MINUTES"); ok {
		p.CoolingOff.BaseMinutes = v
		overridden = true
	}
	if v, ok := modelEnvFloat("FINIX_SIM_MU"); ok {
		p.Simulation.Mu = v
		overridden = true
	}
	if v, ok := modelEnvFloat("FINIX_SIM_SIGMA"); ok {
		p.Simulation.Sigma = v
		overridden = true
	}
	if v, ok := modelEnvFloat("FINIX_EXPECTED_INVESTMENT_RETURN"); ok {
		p.Foreclosure.ExpectedInvestmentReturn = v
		overridden = true
	}
	if v, ok := modelEnvInt("FINIX_COLD_START_MONTHS"); ok {
		p.ColdStartMonths = v
		overridden = true
	}

	if overridden {
		p.Version += "+override"
	}
	return p
}

// Validate rejects an internally inconsistent parameter set at startup.
func (p ModelParams) Validate() error {
	if strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("model params: version is required")
	}
	if !(p.RiskTiers.Medium > 0 && p.RiskTiers.High > p.RiskTiers.Medium && p.RiskTiers.High <= 100) {
		return fmt.Errorf("model params: risk tiers must satisfy 0 < medium < high <= 100 (got %v/%v)", p.RiskTiers.Medium, p.RiskTiers.High)
	}
	if p.CoolingOff.BaseMinutes <= 0 {
		return fmt.Errorf("model params: cooling-off base minutes must be > 0")
	}
	if p.Simulation.Sigma <= 0 || p.Simulation.Count <= 0 {
		return fmt.Errorf("model params: simulation sigma and count must be > 0")
	}
	if p.AmountTerm.FloorAvgPaise <= 0 {
		return fmt.Errorf("model params: amount floor must be > 0 paise")
	}
	if p.HealthScore.RetirementAge <= 0 || p.HealthScore.PostYears <= 0 {
		return fmt.Errorf("model params: retirement age and post years must be > 0")
	}
	if p.Structuring.MinCount <= 0 || p.Structuring.BenchmarkPaise <= 0 {
		return fmt.Errorf("model params: structuring min count and benchmark must be > 0")
	}
	return nil
}

func modelEnvFloat(key string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func modelEnvInt(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}
