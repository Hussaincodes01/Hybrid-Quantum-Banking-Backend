package platform

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Notifications are derived from what the customer actually did.
//
// The app's notifications screen rendered a fixed list — a "₹85,000 transfer",
// a "₹5,000" SIP, a "₹3,00,000 emergency fund" — shown identically to every
// account, including brand-new ones that had done nothing. The backend had a
// notification centre, but nothing ever wrote to it, so it always returned an
// empty array and the screen fell back to its hardcoded list.
//
// Rather than add a second feed endpoint, these derived entries are merged into
// that existing centre, which already has read/dismiss and pagination.

// notificationWindow bounds how far back the feed reaches. Older activity
// stays in the audit log and transaction history; the feed is for what is
// still worth someone's attention.
const notificationWindow = 30 * 24 * time.Hour

// Notifications assembles the feed from real activity: settled payments,
// security events from the audit trail, goal milestones actually reached, and
// premiums or EMIs coming due.
//
// Categories the customer has switched off in settings are excluded, so the
// preferences endpoint that already existed finally does something.
// derivedNotifications builds the feed from real activity. Takes only the main
// read lock, and never the notification lock, so callers may hold that instead.
func (s *Service) derivedNotifications(userID string) []NotificationItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return nil
	}

	now := time.Now()
	cutoff := now.Add(-notificationWindow)
	enabled := s.enabledCategoriesLocked(userID)

	out := make([]NotificationItem, 0, 16)

	if enabled["transactions"] {
		out = append(out, s.transactionNotificationsLocked(userID, cutoff)...)
	}
	if enabled["security"] {
		out = append(out, s.securityNotificationsLocked(userID, cutoff)...)
	}
	if enabled["goals"] {
		out = append(out, s.goalNotificationsLocked(userID, now)...)
	}
	if enabled["insights"] {
		out = append(out, s.dueDateNotificationsLocked(userID, now)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// enabledCategoriesLocked reads the customer's notification preferences. A
// customer with no stored preferences receives everything.
func (s *Service) enabledCategoriesLocked(userID string) map[string]bool {
	settings := s.notifications[userID]
	if len(settings) == 0 {
		return map[string]bool{
			"transactions": true, "security": true,
			"goals": true, "insights": true, "tax": true,
		}
	}
	out := make(map[string]bool, len(settings))
	for _, setting := range settings {
		out[setting.Category] = setting.Enabled
	}
	return out
}

func (s *Service) transactionNotificationsLocked(userID string, cutoff time.Time) []NotificationItem {
	var out []NotificationItem
	for _, t := range s.transactions[userID] {
		if t.CreatedAt.Before(cutoff) {
			continue
		}
		// A payment the customer is still confirming is not news yet.
		if t.Status != "" && t.Status != "success" && t.Status != "completed" {
			continue
		}

		credit := strings.EqualFold(t.DebitCredit, "credit")
		counterparty := firstFilled(t.MerchantName, t.Recipient, "your account")

		title := fmt.Sprintf("Paid %s to %s", formatRupees(t.AmountPaise), counterparty)
		severity := "info"
		if credit {
			title = fmt.Sprintf("Received %s from %s", formatRupees(t.AmountPaise), counterparty)
		}

		body := t.Description
		if body == "" {
			body = fmt.Sprintf("%s · %s", strings.ToUpper(firstNonEmpty(t.Channel, "UPI")), t.Status)
		}
		// A payment the risk engine flagged is worth surfacing differently.
		//
		// RiskScore is on a 0-100 scale, not 0-1: comparing it against 0.7
		// marked every ordinary payment as suspicious. The engine's own level
		// is the reliable signal, with the score only as a fallback for records
		// written before levels were populated.
		if isElevatedRisk(t) {
			severity = "warning"
			if t.XAIReason != "" {
				body = t.XAIReason
			}
		}

		out = append(out, NotificationItem{
			ID:        "ntf_txn_" + t.ID,
			Category:  "transactions",
			Title:     title,
			Body:      body,
			Severity:  severity,
			CreatedAt: t.CreatedAt,
		})
	}
	return out
}

// securityEventTitles maps audit event types worth telling the customer about.
// Anything not listed stays in the audit log rather than becoming a notice —
// the feed should not read like a debug trace.
var securityEventTitles = map[string]struct {
	title    string
	severity string
}{
	"pin_login_success":  {"New sign-in to your account", "info"},
	"pin_login_failure":  {"Failed sign-in attempt", "warning"},
	"emergency_freeze":   {"Account frozen", "critical"},
	"account_unfreeze":   {"Account unfrozen", "warning"},
	"biometric_register": {"Biometric unlock enabled", "info"},
	"device_bound":       {"New device linked", "warning"},
	"beneficiary_added":  {"New payee added", "warning"},
}

func (s *Service) securityNotificationsLocked(userID string, cutoff time.Time) []NotificationItem {
	var out []NotificationItem
	for _, event := range s.audit {
		if event.UserID != userID || event.Timestamp.Before(cutoff) {
			continue
		}
		meta, interesting := securityEventTitles[event.EventType]
		if !interesting {
			continue
		}
		body := firstFilled(event.XAIReason, event.Details, "Reviewed by FINIX security.")
		out = append(out, NotificationItem{
			ID:        "ntf_sec_" + event.ID,
			Category:  "security",
			Title:     meta.title,
			Body:      body,
			CreatedAt: event.Timestamp,
			Severity:  meta.severity,
		})
	}
	return out
}

// goalMilestones are the progress marks worth announcing.
var goalMilestones = []float64{0.25, 0.5, 0.75, 1.0}

func (s *Service) goalNotificationsLocked(userID string, now time.Time) []NotificationItem {
	var out []NotificationItem
	for _, g := range s.goals[userID] {
		if g.TargetAmountPaise <= 0 {
			continue
		}
		progress := float64(g.SavedAmountPaise) / float64(g.TargetAmountPaise)

		// Announce only the highest milestone actually reached, so a goal at
		// 80% produces one notice rather than three.
		reached := -1.0
		for _, m := range goalMilestones {
			if progress >= m {
				reached = m
			}
		}
		if reached < 0 {
			continue
		}

		title := fmt.Sprintf("%s is %d%% funded", g.Name, int(reached*100))
		body := fmt.Sprintf("%s of %s saved.",
			formatRupees(g.SavedAmountPaise), formatRupees(g.TargetAmountPaise))
		if reached >= 1 {
			title = fmt.Sprintf("%s reached", g.Name)
			body = fmt.Sprintf("Fully funded at %s.", formatRupees(g.TargetAmountPaise))
		}

		// Timestamped to the goal's own activity, not to now, so the feed does
		// not reshuffle on every refresh.
		at := g.CreatedAt
		if at.IsZero() {
			at = g.StartDate
		}
		if at.IsZero() || at.After(now) {
			at = now
		}

		out = append(out, NotificationItem{
			ID:        fmt.Sprintf("ntf_goal_%s_%d", g.ID, int(reached*100)),
			Category:  "goals",
			Title:     title,
			Body:      body,
			Severity:  "info",
			CreatedAt: at,
		})
	}
	return out
}

// dueDateNotificationsLocked warns about premiums falling due inside a
// fortnight. EMIs have no stored due date, so they are not invented here.
func (s *Service) dueDateNotificationsLocked(userID string, now time.Time) []NotificationItem {
	var out []NotificationItem
	horizon := now.AddDate(0, 0, 14)

	for _, policy := range s.insurance[userID] {
		due, err := time.Parse("2006-01-02", policy.NextDueDate)
		if err != nil || due.After(horizon) {
			continue
		}
		severity := "info"
		if due.Before(now) {
			severity = "warning"
		}
		out = append(out, NotificationItem{
			ID:       "ntf_prem_" + policy.PolicyID,
			Category: "insights",
			Title: fmt.Sprintf("%s premium due %s",
				policy.Insurer, due.Format("2 Jan")),
			Body:      fmt.Sprintf("%s for your %s policy.", formatRupees(policy.PremiumPaise), readablePolicyType(policy.PolicyType)),
			Severity:  severity,
			CreatedAt: due,
		})
	}
	return out
}

func readablePolicyType(t string) string {
	return strings.ReplaceAll(t, "_", " ")
}

// formatRupees renders paise as rupees with Indian digit grouping.
func formatRupees(paise int64) string {
	if paise < 0 {
		paise = -paise
	}
	rupees := paise / 100
	digits := fmt.Sprintf("%d", rupees)
	if len(digits) <= 3 {
		return "₹" + digits
	}
	last3 := digits[len(digits)-3:]
	rest := digits[:len(digits)-3]
	var groups []string
	for len(rest) > 2 {
		groups = append([]string{rest[len(rest)-2:]}, groups...)
		rest = rest[:len(rest)-2]
	}
	if rest != "" {
		groups = append([]string{rest}, groups...)
	}
	return "₹" + strings.Join(groups, ",") + "," + last3
}

// firstFilled is the variadic form; service.go already defines a two-argument
// firstNonEmpty, which is left alone.
func firstFilled(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// isElevatedRisk reports whether the risk engine singled this payment out.
func isElevatedRisk(t Transaction) bool {
	switch strings.ToLower(string(t.RiskLevel)) {
	case "high", "critical":
		return true
	case "low", "medium":
		return false
	}
	// No level recorded: fall back to the score, which is 0-100.
	return t.RiskScore >= 70
}
