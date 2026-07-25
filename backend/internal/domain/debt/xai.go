package debt

import "fmt"

// rupees formats paise as a rupee string for XAI text.
func rupees(paise int64) string {
	return fmt.Sprintf("₹%.2f", float64(paise)/100.0)
}

// foreclosureXAI states the decision arithmetic including the opportunity-cost
// term explicitly (spec §4.2 requires more than penalty-vs-savings).
func foreclosureXAI(penaltyRate float64, penalty, remainingInterest, opportunityCost, netBenefit int64, loanRate, expReturn float64) string {
	verdict := "Foreclosing is worthwhile"
	if netBenefit < 0 {
		verdict = "Foreclosing is NOT recommended"
	}
	penaltyNote := fmt.Sprintf("penalty %s (rate %.2f%%)", rupees(penalty), penaltyRate*100)
	if penaltyRate == 0 {
		penaltyNote = "no foreclosure penalty (floating-rate retail loan, RBI 2014)"
	}
	return fmt.Sprintf(
		"%s: interest avoided by closing early %s minus %s minus opportunity cost %s "+
			"(the cash could earn %.1f%% invested vs the %.1f%% loan rate) = net %s.",
		verdict, rupees(remainingInterest), penaltyNote, rupees(opportunityCost),
		expReturn*100, loanRate*100, rupees(netBenefit),
	)
}

// optimiseXAI shows BOTH waterfall totals regardless of the recommendation
// (spec §4.3).
func optimiseXAI(recommended string, avalanche, snowball, cost int64, adherence, threshold float64) string {
	reason := "Avalanche clears the most expensive debt first, minimising total interest."
	if recommended == StrategySnowball {
		reason = fmt.Sprintf(
			"Snowball is suggested because your documented adherence (%.2f) is below %.2f, so the "+
				"momentum of clearing small balances first improves follow-through.", adherence, threshold)
	}
	return fmt.Sprintf(
		"%s Avalanche total interest %s vs Snowball %s — choosing snowball costs an extra %s in interest.",
		reason, rupees(avalanche), rupees(snowball), rupees(cost),
	)
}

// repoXAI describes a floating-rate repricing.
func repoXAI(oldRate, newRate float64, oldEMI, newEMI int64) string {
	direction := "increases"
	if newEMI < oldEMI {
		direction = "decreases"
	}
	return fmt.Sprintf(
		"Repo transmission moves your rate from %.2f%% to %.2f%%, so your EMI %s from %s to %s.",
		oldRate, newRate, direction, rupees(oldEMI), rupees(newEMI),
	)
}
