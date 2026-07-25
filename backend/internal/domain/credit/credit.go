// Package credit provides pure credit-utilisation and BNPL math (spec §8).
// No I/O, no state. Money is int64 paise; "now" is passed in so the T-5 lead-time
// check stays deterministic and testable.
package credit

import (
	"strings"
	"time"

	"FINIX/backend/internal/config"
)

// Card is a single revolving credit line.
type Card struct {
	ID               string    `json:"id"`
	OutstandingPaise int64     `json:"outstandingPaise"`
	LimitPaise       int64     `json:"limitPaise"`
	StatementDate    time.Time `json:"statementDate"`
}

// Utilisation is outstanding / limit for one card. A zero/negative limit yields 0.
func Utilisation(outstandingPaise, limitPaise int64) float64 {
	if limitPaise <= 0 {
		return 0
	}
	return float64(outstandingPaise) / float64(limitPaise)
}

// AggregateUtilisation is Σoutstanding / Σlimit across all cards.
func AggregateUtilisation(cards []Card) float64 {
	var totOut, totLimit int64
	for _, c := range cards {
		totOut += c.OutstandingPaise
		totLimit += c.LimitPaise
	}
	if totLimit <= 0 {
		return 0
	}
	return float64(totOut) / float64(totLimit)
}

// CardUtilisation is the per-card breakdown surfaced alongside the aggregate — a
// single maxed card is hidden by a healthy aggregate, so both are reported.
type CardUtilisation struct {
	CardID          string  `json:"cardId"`
	Utilisation     float64 `json:"utilisation"`
	Breached        bool    `json:"breached"`        // utilisation >= alert threshold
	DaysToStatement int     `json:"daysToStatement"` // negative if statement already passed
	AlertInLead     bool    `json:"alertInLead"`     // breached AND within the T-lead window
}

// UtilisationReport bundles per-card detail with the aggregate.
type UtilisationReport struct {
	Aggregate float64           `json:"aggregate"`
	Cards     []CardUtilisation `json:"cards"`
	// AnyAlert is true if at least one card should alert (breached within lead time).
	AnyAlert bool `json:"anyAlert"`
}

// EvaluateUtilisation computes per-card and aggregate utilisation and fires a
// lead-time alert when a card is at/over the threshold within AlertLeadDays BEFORE
// its statement date — the window in which paying down still improves the reported
// utilisation (spec §8).
func EvaluateUtilisation(cards []Card, now time.Time, p config.CreditParams) UtilisationReport {
	rep := UtilisationReport{
		Aggregate: AggregateUtilisation(cards),
		Cards:     make([]CardUtilisation, 0, len(cards)),
	}
	for _, c := range cards {
		u := Utilisation(c.OutstandingPaise, c.LimitPaise)
		days := daysBetween(now, c.StatementDate)
		breached := u >= p.UtilisationAlert
		inLead := breached && days >= 0 && days <= p.AlertLeadDays
		if inLead {
			rep.AnyAlert = true
		}
		rep.Cards = append(rep.Cards, CardUtilisation{
			CardID:          c.ID,
			Utilisation:     u,
			Breached:        breached,
			DaysToStatement: days,
			AlertInLead:     inLead,
		})
	}
	return rep
}

// daysBetween returns whole days from now to future (floored).
func daysBetween(now, future time.Time) int {
	return int(future.Sub(now).Hours() / 24)
}

// EffectiveAPR converts a "0% headline" repayment into a true annualised cost:
//
//	APR = ((totalRepayable/principal) − 1) × (365/tenureDays).
//
// A genuinely-free BNPL (totalRepayable == principal) is 0%; any processing or
// convenience fee makes it materially positive. Returns 0 on invalid input.
func EffectiveAPR(totalRepayablePaise, principalPaise int64, tenureDays int) float64 {
	if principalPaise <= 0 || tenureDays <= 0 {
		return 0
	}
	costRatio := float64(totalRepayablePaise)/float64(principalPaise) - 1.0
	return costRatio * (365.0 / float64(tenureDays))
}

// IsBNPLObligation classifies a transaction as a BNPL settlement by MCC or by a
// counterparty name matching the versioned provider registry.
func IsBNPLObligation(mcc, counterparty string, p config.CreditParams) bool {
	mcc = strings.TrimSpace(mcc)
	for _, code := range p.BNPLMerchantCategoryCodes {
		if mcc == code {
			return true
		}
	}
	name := strings.ToLower(strings.TrimSpace(counterparty))
	if name == "" {
		return false
	}
	for _, provider := range p.BNPLProviders {
		if provider != "" && strings.Contains(name, provider) {
			return true
		}
	}
	return false
}

// Obligation is one outstanding BNPL instalment liability.
type Obligation struct {
	Provider       string    `json:"provider"`
	DueAmountPaise int64     `json:"dueAmountPaise"`
	DueDate        time.Time `json:"dueDate"`
}

// TotalBNPLOutstanding sums BNPL principal outstanding so the health-score DTI (P6)
// and the debt engine (P2) include it as real debt rather than ignoring it.
func TotalBNPLOutstanding(obligations []Obligation) int64 {
	var total int64
	for _, o := range obligations {
		if o.DueAmountPaise > 0 {
			total += o.DueAmountPaise
		}
	}
	return total
}
