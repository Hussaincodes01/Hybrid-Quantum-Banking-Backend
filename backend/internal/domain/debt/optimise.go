package debt

import (
	"math"
	"sort"

	"FINIX/backend/internal/config"
)

// Loan is the pure input to the optimisation functions. The service maps its
// LoanRecord into this; the debt package never touches service state.
type Loan struct {
	ID             string
	PrincipalPaise int64 // current outstanding principal
	AnnualRatePct  float64
	Months         int   // remaining months
	EMIPaise       int64 // current EMI; if 0, derived from the other fields
	Floating       bool
	LoanType       string // home | car | personal | education | business | gold | ...
}

// emi returns the loan's EMI, deriving it when not supplied.
func (l Loan) emi() int64 {
	if l.EMIPaise > 0 {
		return l.EMIPaise
	}
	return EMI(l.PrincipalPaise, l.AnnualRatePct, l.Months)
}

// prepaySchedule amortises principal at a fixed EMI, applying prepayments at the
// start of the given month (as extra principal), until the loan closes. If the
// balance cannot clear within monthCap (or the EMI can't cover interest) the
// returned schedule ends with a non-zero final balance; callers detect this via
// scheduleConverged rather than a separate flag.
func prepaySchedule(principalPaise int64, annualRatePct float64, emiPaise int64, prepayments map[int]int64) []Row {
	if principalPaise <= 0 || emiPaise <= 0 {
		return nil
	}
	r := monthlyRate(annualRatePct)
	balance := principalPaise
	rows := make([]Row, 0, 512)
	const monthCap = 1200 // 100 years — safety bound
	for k := 1; k <= monthCap && balance > 0; k++ {
		if extra, ok := prepayments[k]; ok && extra > 0 {
			if extra > balance {
				extra = balance
			}
			balance -= extra
			if balance == 0 {
				rows = append(rows, Row{Month: k, InterestPaise: 0, PrincipalPaise: extra, BalancePaise: 0})
				break
			}
		}
		interest := roundPaise(float64(balance) * r)
		principal := emiPaise - interest
		if principal <= 0 {
			// EMI can't cover interest — loan never amortises; bail out.
			return rows
		}
		if principal >= balance {
			principal = balance
		}
		balance -= principal
		rows = append(rows, Row{Month: k, InterestPaise: interest, PrincipalPaise: principal, BalancePaise: balance})
	}
	return rows
}

// PrepaymentResult is the outcome of a lump-sum prepayment.
type PrepaymentResult struct {
	InterestSavedPaise  int64 `json:"interestSavedPaise"`
	TenureReducedMonths int   `json:"tenureReducedMonths"`
	BaselineMonths      int   `json:"baselineMonths"`
	NewMonths           int   `json:"newMonths"`
	BaselineInterest    int64 `json:"baselineInterestPaise"`
	NewInterest         int64 `json:"newInterestPaise"`
	// Converged is false when the simulation hit the 1200-month safety bound
	// without the balance clearing. Callers should treat the result as an upper
	// bound, not a precise payoff schedule.
	Converged bool `json:"converged"`
}

// PrepaymentImpact runs the amortisation twice (baseline vs. one lump-sum extra at
// extraMonth, re-amortised at the SAME EMI) and returns the real interest saved
// and tenure reduction — replacing the old `extra*14` / `11` literals.
func PrepaymentImpact(loan Loan, extraPaise int64, extraMonth int) PrepaymentResult {
	emi := loan.emi()
	if extraMonth < 1 {
		extraMonth = 1
	}
	base := prepaySchedule(loan.PrincipalPaise, loan.AnnualRatePct, emi, nil)
	prepay := prepaySchedule(loan.PrincipalPaise, loan.AnnualRatePct, emi, map[int]int64{extraMonth: extraPaise})

	baseInt := TotalInterest(base)
	newInt := TotalInterest(prepay)
	saved := baseInt - newInt
	if saved < 0 {
		saved = 0
	}
	reduced := len(base) - len(prepay)
	if reduced < 0 {
		reduced = 0
	}
	// Converged = false when either schedule hit the 1200-month safety bound.
	converged := scheduleConverged(base) && scheduleConverged(prepay)
	return PrepaymentResult{
		InterestSavedPaise:  saved,
		TenureReducedMonths: reduced,
		BaselineMonths:      len(base),
		NewMonths:           len(prepay),
		BaselineInterest:    baseInt,
		NewInterest:         newInt,
		Converged:           converged,
	}
}

// scheduleConverged reports whether a schedule ended with a zero balance.
func scheduleConverged(rows []Row) bool {
	if len(rows) == 0 {
		return true
	}
	return rows[len(rows)-1].BalancePaise == 0
}

// isIndividualRetail reports whether a loan type is an individual retail loan (as
// opposed to a business loan) for the RBI floating-rate no-penalty rule.
func isIndividualRetail(loanType string) bool {
	switch loanType {
	case "business", "commercial", "sme", "msme":
		return false
	default:
		return true
	}
}

// ForeclosurePenaltyRate returns the foreclosure/prepayment penalty rate.
//
// COMPLIANCE (RBI 2014 circular): floating-rate individual retail loans MUST have
// a ZERO penalty. The old handler hardcoded has_penalty:true, which is incorrect.
// Fixed-rate loans and business loans use the versioned lender rate.
func ForeclosurePenaltyRate(loanType string, floating bool, p config.ForeclosureParams) float64 {
	if floating && isIndividualRetail(loanType) {
		return 0
	}
	return p.FixedRateLenderPenaltyRate
}

// ForeclosureResult carries the full economic breakdown of a foreclosure decision.
type ForeclosureResult struct {
	HasPenalty             bool    `json:"hasPenalty"`
	PenaltyRate            float64 `json:"penaltyRate"`
	PenaltyAmountPaise     int64   `json:"penaltyAmountPaise"`
	RemainingInterestPaise int64   `json:"remainingInterestPaise"`
	OpportunityCostPaise   int64   `json:"opportunityCostPaise"`
	NetBenefitPaise        int64   `json:"netBenefitPaise"`
	Recommended            bool    `json:"recommended"`
	Explanation            string  `json:"explanation"`
}

// ForeclosureDecision computes:
//
//	NetBenefit = RemainingInterestIfHeld − Penalty − OpportunityCost
//	OpportunityCost = amountUsed × (expectedInvestmentReturn − loanRate) × remainingYears
//
// The opportunity-cost term is surfaced explicitly: even with a zero penalty,
// foreclosing a cheap loan (rate below the expected investment return) can be a
// net loss because the cash would earn more if invested.
func ForeclosureDecision(loan Loan, amountUsedPaise int64, p config.ForeclosureParams) ForeclosureResult {
	penaltyRate := ForeclosurePenaltyRate(loan.LoanType, loan.Floating, p)
	penalty := roundPaise(float64(loan.PrincipalPaise) * penaltyRate)

	remainingInterest := TotalInterest(Schedule(loan.PrincipalPaise, loan.AnnualRatePct, loan.Months))

	loanRate := loan.AnnualRatePct / 100.0
	remainingYears := float64(loan.Months) / 12.0
	opportunityCost := roundPaise(float64(amountUsedPaise) * (p.ExpectedInvestmentReturn - loanRate) * remainingYears)

	netBenefit := remainingInterest - penalty - opportunityCost
	recommended := netBenefit >= 0

	expl := foreclosureXAI(penaltyRate, penalty, remainingInterest, opportunityCost, netBenefit, loanRate, p.ExpectedInvestmentReturn)
	return ForeclosureResult{
		HasPenalty:             penaltyRate > 0,
		PenaltyRate:            penaltyRate,
		PenaltyAmountPaise:     penalty,
		RemainingInterestPaise: remainingInterest,
		OpportunityCostPaise:   opportunityCost,
		NetBenefitPaise:        netBenefit,
		Recommended:            recommended,
		Explanation:            expl,
	}
}

// Strategy names.
const (
	StrategyAvalanche = "avalanche"
	StrategySnowball  = "snowball"
)

// OptimiseResult reports both waterfalls so the client can always see the trade-off.
type OptimiseResult struct {
	AvalancheInterestPaise int64  `json:"avalancheInterestPaise"`
	SnowballInterestPaise  int64  `json:"snowballInterestPaise"`
	InterestCostOfSnowball int64  `json:"interestCostOfSnowballPaise"` // snowball − avalanche, always ≥ 0
	AvalancheMonths        int    `json:"avalancheMonths"`
	SnowballMonths         int    `json:"snowballMonths"`
	RecommendedStrategy    string `json:"recommendedStrategy"`
	Explanation            string `json:"explanation"`
	// Converged is false when either waterfall hit the 1200-month safety bound.
	Converged bool `json:"converged"`
}

// OptimiseStrategy simulates both debt waterfalls with the same constant monthly
// outlay (sum of EMIs + extraTarget) and returns both total-interest figures.
// Per §4.3 the recommendation is Avalanche (lowest interest) unless adherence is
// documented-low, where Snowball's behavioural momentum can justify its cost.
func OptimiseStrategy(loans []Loan, extraTargetPaise int64, adherenceScore float64, p config.DebtParams) OptimiseResult {
	avInt, avMonths, avConv := simulateWaterfall(loans, extraTargetPaise, StrategyAvalanche)
	snInt, snMonths, snConv := simulateWaterfall(loans, extraTargetPaise, StrategySnowball)

	cost := snInt - avInt
	if cost < 0 {
		cost = 0 // avalanche is provably never worse; guard rounding
	}

	recommended := StrategyAvalanche
	if adherenceScore > 0 && adherenceScore < p.LowAdherenceThreshold {
		recommended = StrategySnowball
	}
	return OptimiseResult{
		AvalancheInterestPaise: avInt,
		SnowballInterestPaise:  snInt,
		InterestCostOfSnowball: cost,
		AvalancheMonths:        avMonths,
		SnowballMonths:         snMonths,
		RecommendedStrategy:    recommended,
		Explanation:            optimiseXAI(recommended, avInt, snInt, cost, adherenceScore, p.LowAdherenceThreshold),
		Converged:              avConv && snConv,
	}
}

type acct struct {
	bal  int64
	r    float64
	emi  int64
	rate float64
}

// simulateWaterfall runs a rollover debt waterfall and returns (totalInterest,
// months, converged). The monthly outlay is constant: each active loan pays its
// EMI, and the extra budget plus any EMIs freed by closed loans is poured onto
// the priority loan (highest rate for avalanche, lowest balance for snowball).
func simulateWaterfall(loans []Loan, extra int64, strategy string) (int64, int, bool) {
	accts := make([]acct, 0, len(loans))
	for _, l := range loans {
		if l.PrincipalPaise <= 0 {
			continue
		}
		accts = append(accts, acct{
			bal:  l.PrincipalPaise,
			r:    monthlyRate(l.AnnualRatePct),
			emi:  l.emi(),
			rate: l.AnnualRatePct,
		})
	}
	if len(accts) == 0 {
		return 0, 0, true
	}

	var totalInterest int64
	const monthCap = 1200
	month := 0
	for month < monthCap {
		if !anyActive(accts) {
			break
		}
		month++

		// 1. Each active loan accrues interest and pays its EMI.
		for i := range accts {
			if accts[i].bal <= 0 {
				continue
			}
			interest := roundPaise(float64(accts[i].bal) * accts[i].r)
			totalInterest += interest
			principal := accts[i].emi - interest
			if principal <= 0 {
				continue // EMI can't cover interest this month
			}
			if principal > accts[i].bal {
				principal = accts[i].bal
			}
			accts[i].bal -= principal
		}

		// 2. Pool = extra + EMIs freed by loans that are now closed.
		pool := extra
		for i := range accts {
			if accts[i].bal <= 0 {
				pool += accts[i].emi
			}
		}

		// 3. Pour the pool onto the priority active loan (as principal prepayment).
		if target := pickTarget(accts, strategy); target >= 0 && pool > 0 {
			pay := pool
			if pay > accts[target].bal {
				pay = accts[target].bal
			}
			accts[target].bal -= pay
		}
	}
	converged := !anyActive(accts) // false only when monthCap was hit with balances remaining
	return totalInterest, month, converged
}

func anyActive(accts []acct) bool {
	for i := range accts {
		if accts[i].bal > 0 {
			return true
		}
	}
	return false
}

// pickTarget returns the index of the priority active loan, or -1 if none active.
func pickTarget(accts []acct, strategy string) int {
	best := -1
	for i := range accts {
		if accts[i].bal <= 0 {
			continue
		}
		if best == -1 {
			best = i
			continue
		}
		if strategy == StrategySnowball {
			if accts[i].bal < accts[best].bal {
				best = i
			}
		} else { // avalanche
			if accts[i].rate > accts[best].rate {
				best = i
			}
		}
	}
	return best
}

// sortLoansCopy returns loans sorted for display (avalanche: rate desc). Exposed
// so a caller can present the payoff order without re-deriving it.
func SortForStrategy(loans []Loan, strategy string) []Loan {
	out := append([]Loan(nil), loans...)
	sort.SliceStable(out, func(i, j int) bool {
		if strategy == StrategySnowball {
			return out[i].PrincipalPaise < out[j].PrincipalPaise
		}
		return out[i].AnnualRatePct > out[j].AnnualRatePct
	})
	return out
}

// RepoResult reports a floating-rate repricing.
type RepoResult struct {
	OldRatePct  float64 `json:"oldRatePct"`
	NewRatePct  float64 `json:"newRatePct"`
	OldEMIPaise int64   `json:"oldEmiPaise"`
	NewEMIPaise int64   `json:"newEmiPaise"`
	Explanation string  `json:"explanation"`
}

// RepoRateRecompute applies a repo-rate change to a floating loan and recomputes
// the EMI. deltaRepoBps is in basis points (100 bps = 1 percentage point); the
// loan rate is in percentage points, so newRate = old + (bps/100)·transmission.
// Replaces the hardcoded 2915000 EMI literal.
func RepoRateRecompute(loan Loan, deltaRepoBps float64, transmissionFactor float64) RepoResult {
	newRatePct := loan.AnnualRatePct + (deltaRepoBps/100.0)*transmissionFactor
	if newRatePct < 0 {
		newRatePct = 0
	}
	oldEMI := loan.emi()
	newEMI := EMI(loan.PrincipalPaise, newRatePct, loan.Months)
	return RepoResult{
		OldRatePct:  loan.AnnualRatePct,
		NewRatePct:  round2(newRatePct),
		OldEMIPaise: oldEMI,
		NewEMIPaise: newEMI,
		Explanation: repoXAI(loan.AnnualRatePct, newRatePct, oldEMI, newEMI),
	}
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
