package healthscore

import (
	"math"
	"time"
)

// Facts is everything about one customer that the score depends on, in plain
// values. The service fills it from its own state; nothing here knows about
// storage, HTTP or locking, so the whole derivation is unit-testable.
//
// This exists because every pillar input except debt-to-income used to be a
// literal in the service: emergency fund 4.5 months, savings rate 0.24,
// diversification 72, protection 68, behaviour 74, and a fixed six-month
// history. Every customer therefore scored within a point or two of the same
// number, and the score never moved when they actually transacted.
type Facts struct {
	Now time.Time

	// Cash reachable today, across linked accounts.
	LiquidBalancePaise int64

	Transactions []TxnFact
	Investments  []HoldingFact
	Goals        []GoalFact

	// Total sum assured across active policies.
	InsuranceCoverPaise int64

	// Outstanding borrowings and the monthly servicing cost of them.
	LiabilitiesPaise int64
	MonthlyEMIPaise  int64
}

type TxnFact struct {
	AmountPaise int64
	Credit      bool
	At          time.Time
}

type HoldingFact struct {
	Category          string
	CurrentValuePaise int64
}

type GoalFact struct {
	SavedPaise  int64
	TargetPaise int64
	Priority    string
	StartDate   time.Time
	TargetDate  time.Time
}

// analysisWindow is how far back the averages look. Six months matches the
// resilience modifier's six-slot history.
const analysisWindow = 6

// Derive turns raw customer facts into the calculator's inputs.
//
// Every value is bounded and every division guarded: a brand-new customer with
// no transactions must produce a defensible score rather than a NaN or a
// divide-by-zero panic.
func Derive(f Facts) Input {
	monthly := monthlyBuckets(f.Transactions, f.Now)

	avgSpend := averageDebits(monthly)
	avgIncome := averageCredits(monthly)

	return Input{
		EmergencyFundMonths:     emergencyFundMonths(f.LiquidBalancePaise, avgSpend),
		DebtToIncomeRatio:       debtToIncome(f.MonthlyEMIPaise, avgIncome, f.LiabilitiesPaise, f.LiquidBalancePaise),
		SavingsRate:             savingsRate(avgIncome, avgSpend),
		DiversificationScore:    diversification(f.Investments),
		InsuranceCoverageScore:  protection(f.InsuranceCoverPaise, avgIncome, f.LiabilitiesPaise),
		GoalOnTrackRatio:        goalOnTrack(f.Goals, f.Now),
		BehaviourQualityScore:   behaviourQuality(monthly),
		MonthlyHistoricalScores: monthlyHistory(monthly),
	}
}

// bucket is one calendar month of activity.
type bucket struct {
	key            string
	creditsPaise   int64
	debitsPaise    int64
	debitCount     int
	hasObservation bool
}

// monthlyBuckets groups transactions into the trailing [analysisWindow] months,
// oldest first. Months with no activity are retained as empty buckets so a gap
// in spending is visible rather than silently skipped.
func monthlyBuckets(txns []TxnFact, now time.Time) []bucket {
	if now.IsZero() {
		now = time.Now()
	}
	buckets := make([]bucket, analysisWindow)
	index := map[string]int{}
	for i := 0; i < analysisWindow; i++ {
		m := now.AddDate(0, -(analysisWindow - 1 - i), 0)
		key := m.Format("2006-01")
		buckets[i] = bucket{key: key}
		index[key] = i
	}

	for _, t := range txns {
		if t.At.IsZero() || t.At.After(now) {
			continue
		}
		i, ok := index[t.At.Format("2006-01")]
		if !ok {
			continue // older than the window
		}
		amount := abs64(t.AmountPaise)
		buckets[i].hasObservation = true
		if t.Credit {
			buckets[i].creditsPaise += amount
		} else {
			buckets[i].debitsPaise += amount
			buckets[i].debitCount++
		}
	}
	return buckets
}

// averageDebits/averageCredits average only over months that actually saw
// activity. Averaging across empty months would halve a new customer's apparent
// spend and inflate their emergency-fund cover.
func averageDebits(buckets []bucket) float64 {
	total, n := 0.0, 0
	for _, b := range buckets {
		if !b.hasObservation {
			continue
		}
		total += float64(b.debitsPaise)
		n++
	}
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

func averageCredits(buckets []bucket) float64 {
	total, n := 0.0, 0
	for _, b := range buckets {
		if !b.hasObservation {
			continue
		}
		total += float64(b.creditsPaise)
		n++
	}
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

// emergencyFundMonths is how long liquid cash covers typical spending.
//
// With no spending history there is nothing to divide by; report the target of
// six months rather than infinity, so an inactive account is not scored as if
// it were in crisis.
func emergencyFundMonths(liquidPaise int64, avgMonthlySpend float64) float64 {
	if avgMonthlySpend <= 0 {
		if liquidPaise > 0 {
			return 6
		}
		return 0
	}
	return clamp(float64(liquidPaise)/avgMonthlySpend, 0, 24)
}

// debtToIncome is the real ratio the pillar claims to measure: what share of
// monthly income goes to servicing debt.
//
// The service previously used debt/(assets+debt), which is leverage, not DTI —
// it made an asset-rich borrower look highly indebted. Where no income has been
// observed, fall back to that leverage measure rather than reporting zero debt.
func debtToIncome(monthlyEMIPaise int64, avgMonthlyIncome float64, liabilitiesPaise, liquidPaise int64) float64 {
	if avgMonthlyIncome > 0 {
		return clamp(float64(monthlyEMIPaise)/avgMonthlyIncome, 0, 1)
	}
	assets := float64(liquidPaise)
	debt := float64(liabilitiesPaise)
	if assets+debt <= 0 {
		return 0
	}
	return clamp(debt/(assets+debt), 0, 1)
}

// neutralSavingsRate is what to report when no income has been observed.
//
// Returning 0 would score the pillar at zero, which asserts the customer saves
// nothing. The truth is that their salary does not land in an account we can
// see — an absence of evidence, not evidence of overspending. This value maps
// through ratioScore(x, savingsRateTarget) to 60/100, the same neutral mark
// behaviourQuality uses when it lacks history.
const (
	savingsRateTarget  = 0.35
	neutralSavingsRate = savingsRateTarget * 0.6
)

// savingsRate is the share of income not spent.
func savingsRate(avgIncome, avgSpend float64) float64 {
	if avgIncome <= 0 {
		return neutralSavingsRate
	}
	return clamp((avgIncome-avgSpend)/avgIncome, 0, 1)
}

// diversification rewards holding several asset classes without letting one
// dominate, scored 0-100.
//
// Concentration is measured with a Herfindahl index over category weights: a
// single holding scores 1, an even spread across n categories scores 1/n. The
// score blends how many classes are held with how evenly they are weighted, so
// five funds all in equity do not read as diversified.
func diversification(holdings []HoldingFact) float64 {
	byCategory := map[string]int64{}
	var total int64
	for _, h := range holdings {
		if h.CurrentValuePaise <= 0 {
			continue
		}
		category := h.Category
		if category == "" {
			category = "other"
		}
		byCategory[category] += h.CurrentValuePaise
		total += h.CurrentValuePaise
	}
	if total <= 0 || len(byCategory) == 0 {
		return 0
	}

	hhi := 0.0
	for _, v := range byCategory {
		w := float64(v) / float64(total)
		hhi += w * w
	}
	// evenness: 0 when everything sits in one category, approaching 1 as the
	// split becomes uniform across the categories held.
	evenness := 0.0
	if n := float64(len(byCategory)); n > 1 {
		evenness = (1 - hhi) / (1 - 1/n)
	}
	// breadth: four or more distinct classes is treated as full marks.
	breadth := clamp(float64(len(byCategory))/4, 0, 1)

	return clamp(100*(0.6*breadth+0.4*evenness), 0, 100)
}

// protection scores life and health cover against what it has to replace:
// outstanding debt plus roughly ten years of income, the usual rule of thumb.
func protection(coverPaise int64, avgMonthlyIncome float64, liabilitiesPaise int64) float64 {
	required := float64(liabilitiesPaise) + avgMonthlyIncome*12*10
	if required <= 0 {
		// Nothing to protect against: no debt and no observed income. Holding
		// any cover at all is adequate; holding none is not penalised harshly.
		if coverPaise > 0 {
			return 100
		}
		return 50
	}
	return clamp(float64(coverPaise)/required*100, 0, 100)
}

// goalOnTrack compares each goal's actual progress with where it should be by
// now on a straight line from start to target date, weighted by priority.
func goalOnTrack(goals []GoalFact, now time.Time) float64 {
	if len(goals) == 0 {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}

	weighted, weightSum := 0.0, 0.0
	for _, g := range goals {
		if g.TargetPaise <= 0 {
			continue
		}
		actual := clamp(float64(g.SavedPaise)/float64(g.TargetPaise), 0, 1)

		expected := 1.0
		if g.TargetDate.After(g.StartDate) && !g.StartDate.IsZero() {
			total := g.TargetDate.Sub(g.StartDate).Seconds()
			elapsed := now.Sub(g.StartDate).Seconds()
			expected = clamp(elapsed/total, 0, 1)
		}

		ratio := 1.0
		if expected > 0 {
			ratio = clamp(actual/expected, 0, 1)
		}

		w := priorityWeight(g.Priority)
		weighted += ratio * w
		weightSum += w
	}
	if weightSum == 0 {
		return 0
	}
	return clamp(weighted/weightSum, 0, 1)
}

func priorityWeight(priority string) float64 {
	switch priority {
	case "high":
		return 1.5
	case "low":
		return 0.6
	default:
		return 1
	}
}

// behaviourQuality rewards consistent spending. It is the coefficient of
// variation of monthly outflow, inverted: steady spending scores high, months
// that swing wildly against the customer's own average score low.
func behaviourQuality(buckets []bucket) float64 {
	values := make([]float64, 0, len(buckets))
	for _, b := range buckets {
		if b.hasObservation {
			values = append(values, float64(b.debitsPaise))
		}
	}
	if len(values) < 2 {
		// Not enough history to judge consistency either way. Neutral, rather
		// than rewarding or punishing a new customer for having no record.
		return 60
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	if mean <= 0 {
		return 60
	}

	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	cv := math.Sqrt(variance) / mean

	// cv of 0 is perfectly steady; 0.6 or worse is erratic.
	return clamp((1-clamp(cv/0.6, 0, 1))*100, 0, 100)
}

// monthlyHistory produces the six-month trend the resilience modifier reads.
// Each month is scored on how much of that month's income was retained, which
// is the part of the score a customer moves month to month.
func monthlyHistory(buckets []bucket) []float64 {
	out := make([]float64, 0, len(buckets))
	for _, b := range buckets {
		if !b.hasObservation || b.creditsPaise <= 0 {
			// No income recorded that month: neutral rather than zero, which
			// would drag the modifier down for a gap in the data.
			out = append(out, 60)
			continue
		}
		retained := (float64(b.creditsPaise) - float64(b.debitsPaise)) / float64(b.creditsPaise)
		out = append(out, clamp(retained*100, 0, 100))
	}
	// Already chronological: monthlyBuckets builds oldest-first, which is the
	// order resilienceModifier's ascending weights expect.
	return out
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
