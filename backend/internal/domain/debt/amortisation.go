// Package debt provides pure, deterministic loan amortisation and optimisation
// math (spec §4, "Srishti Math"). No I/O, no mutexes, no state — every function
// is a pure transform so the results are reproducible for RBI model-risk audit.
//
// Money is int64 paise throughout. Floating point is used only for intermediate
// rate arithmetic and is rounded back to paise at each recorded step.
package debt

import "math"

// Row is one line of an amortisation schedule.
type Row struct {
	Month         int   `json:"month"`
	InterestPaise int64 `json:"interestPaise"`
	// PrincipalPaise is the principal repaid this month.
	PrincipalPaise int64 `json:"principalPaise"`
	// BalancePaise is the outstanding principal AFTER this month's payment.
	BalancePaise int64 `json:"balancePaise"`
}

// monthlyRate converts an annual percentage rate to a monthly decimal rate.
// annualRatePct is in percentage points (9.0 == 9%).
func monthlyRate(annualRatePct float64) float64 {
	return annualRatePct / 12.0 / 100.0
}

func roundPaise(f float64) int64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int64(math.Round(f))
}

// EMI computes the level monthly instalment for a loan using
//
//	EMI = P·r·(1+r)^n / ((1+r)^n − 1),  r = annualRatePct/12/100.
//
// r == 0 degenerates to P/n. months <= 0 or principal <= 0 returns 0. The result
// is rounded to whole paise.
func EMI(principalPaise int64, annualRatePct float64, months int) int64 {
	if months <= 0 || principalPaise <= 0 {
		return 0
	}
	r := monthlyRate(annualRatePct)
	p := float64(principalPaise)
	if r == 0 {
		return roundPaise(p / float64(months))
	}
	pow := math.Pow(1+r, float64(months))
	emi := p * r * pow / (pow - 1)
	return roundPaise(emi)
}

// Schedule simulates the full amortisation table month by month:
//
//	interest(k)  = balance(k-1)·r
//	principal(k) = EMI − interest(k)
//	balance(k)   = balance(k-1) − principal(k)
//
// The final row absorbs rounding residue so the closing balance is exactly 0
// (standard lender practice — the last instalment is adjusted). If the EMI does
// not cover the monthly interest (balance can never amortise) it returns nil.
func Schedule(principalPaise int64, annualRatePct float64, months int) []Row {
	if months <= 0 || principalPaise <= 0 {
		return nil
	}
	emi := EMI(principalPaise, annualRatePct, months)
	if emi <= 0 {
		return nil
	}
	r := monthlyRate(annualRatePct)

	// If the instalment can't cover the first month's interest, the loan never
	// amortises — reject rather than loop forever.
	if r > 0 {
		firstInterest := roundPaise(float64(principalPaise) * r)
		if emi <= firstInterest {
			return nil
		}
	}

	rows := make([]Row, 0, months)
	balance := principalPaise
	for k := 1; k <= months && balance > 0; k++ {
		interest := roundPaise(float64(balance) * r)
		principal := emi - interest
		// On the final scheduled month, or if this instalment would overshoot,
		// pay off exactly the remaining balance.
		if k == months || principal >= balance {
			principal = balance
			interest = roundPaise(float64(balance) * r)
		}
		balance -= principal
		rows = append(rows, Row{
			Month:          k,
			InterestPaise:  interest,
			PrincipalPaise: principal,
			BalancePaise:   balance,
		})
	}
	return rows
}

// TotalInterest sums the interest across a schedule exactly (no approximation).
func TotalInterest(rows []Row) int64 {
	var total int64
	for _, row := range rows {
		total += row.InterestPaise
	}
	return total
}

// TotalPrincipal sums the principal repaid across a schedule.
func TotalPrincipal(rows []Row) int64 {
	var total int64
	for _, row := range rows {
		total += row.PrincipalPaise
	}
	return total
}

// SolveTenure recomputes the number of months required to clear a principal at a
// given EMI and rate — used for floating-rate repricing where the EMI is held
// constant and the tenure moves:
//
//	n = −ln(1 − P·r/EMI) / ln(1+r).
//
// r == 0 degenerates to ceil(P/EMI). Returns 0 when the EMI cannot cover the
// monthly interest (tenure would be infinite) or on invalid input.
func SolveTenure(principalPaise int64, annualRatePct float64, emiPaise int64) int {
	if principalPaise <= 0 || emiPaise <= 0 {
		return 0
	}
	r := monthlyRate(annualRatePct)
	p := float64(principalPaise)
	e := float64(emiPaise)
	if r == 0 {
		return int(math.Ceil(p / e))
	}
	// EMI must exceed the first month's interest or the balance never shrinks.
	if e <= p*r {
		return 0
	}
	n := -math.Log(1-p*r/e) / math.Log(1+r)
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0
	}
	return int(math.Ceil(n))
}
