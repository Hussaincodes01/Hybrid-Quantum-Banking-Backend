package platform

import (
	"context"
	"fmt"
	"math"
	mrand "math/rand"
	"strconv"
	"strings"
	"time"

	"FINIX/backend/internal/domain/transaction"
)

type BehaviourPrediction struct {
	UserID                    string             `json:"userId"`
	Segment                   string             `json:"segment"`
	SecurityRiskScore         float64            `json:"securityRiskScore"`
	FraudProbability          float64            `json:"fraudProbability"`
	BehaviourStabilityScore   float64            `json:"behaviourStabilityScore"`
	SpendingDisciplineScore   float64            `json:"spendingDisciplineScore"`
	GoalCompletionProbability float64            `json:"goalCompletionProbability"`
	RecommendedNudge          string             `json:"recommendedNudge"`
	FeatureVector             map[string]float64 `json:"featureVector"`
	GeneratedAt               time.Time          `json:"generatedAt"`
}

type GoalFundingRecommendation struct {
	GoalID                           string  `json:"goalId"`
	GoalName                         string  `json:"goalName"`
	RemainingAmountPaise             int64   `json:"remainingAmountPaise"`
	RequiredMonthlyContributionPaise int64   `json:"requiredMonthlyContributionPaise"`
	MonthsLeft                       int     `json:"monthsLeft"`
	Priority                         string  `json:"priority"`
	ProjectedCompletionProbability   float64 `json:"projectedCompletionProbability"`
}

type InvestmentRecommendation struct {
	UserID                   string                      `json:"userId"`
	RiskProfile              string                      `json:"riskProfile"`
	MonthlySIPPaise          int64                       `json:"monthlySipPaise"`
	EmergencyFundTargetPaise int64                       `json:"emergencyFundTargetPaise"`
	SuggestedAssetSplit      map[string]float64          `json:"suggestedAssetSplit"`
	GoalRecommendations      []GoalFundingRecommendation `json:"goalRecommendations"`
	RebalanceActions         []string                    `json:"rebalanceActions"`
	Explainability           []string                    `json:"explainability"`
	GeneratedAt              time.Time                   `json:"generatedAt"`
}

type SyntheticDatasetSpec struct {
	UserCount int   `json:"userCount"`
	Days      int   `json:"days"`
	Seed      int64 `json:"seed"`
}

type SyntheticBehaviourRow struct {
	SyntheticUserID           string  `json:"syntheticUserId"`
	MonthlyIncomePaise        int64   `json:"monthlyIncomePaise"`
	MonthlyExpensePaise       int64   `json:"monthlyExpensePaise"`
	SavingsRate               float64 `json:"savingsRate"`
	TransactionCount          int     `json:"transactionCount"`
	LateNightTransactionRatio float64 `json:"lateNightTransactionRatio"`
	NewBeneficiaryRatio       float64 `json:"newBeneficiaryRatio"`
	FailedAuthAttempts        int     `json:"failedAuthAttempts"`
	BehaviourDriftScore       float64 `json:"behaviourDriftScore"`
	FraudRiskProbability      float64 `json:"fraudRiskProbability"`
	FraudLabel                bool    `json:"fraudLabel"`
	SIPAffinityScore          float64 `json:"sipAffinityScore"`
}

type SyntheticDataset struct {
	Spec        SyntheticDatasetSpec    `json:"spec"`
	Rows        []SyntheticBehaviourRow `json:"rows"`
	Summary     map[string]any          `json:"summary"`
	GeneratedAt time.Time               `json:"generatedAt"`
}

type BankAPIReadiness struct {
	Status            string            `json:"status"`
	ContractVersion   string            `json:"contractVersion"`
	OpenStandards     []string          `json:"openStandards"`
	AuthModes         []string          `json:"authModes"`
	RequiredEndpoints []string          `json:"requiredEndpoints"`
	WebhookEvents     []string          `json:"webhookEvents"`
	FieldMapping      map[string]string `json:"fieldMapping"`
	IntegrationNotes  []string          `json:"integrationNotes"`
}

type BankConnectionRequest struct {
	Provider      string `json:"provider"`
	BaseURL       string `json:"baseUrl"`
	ClientID      string `json:"clientId"`
	WebhookSecret string `json:"webhookSecret"`
	Sandbox       bool   `json:"sandbox"`
}

type BankConnectionStatus struct {
	ConnectionID    string    `json:"connectionId"`
	Provider        string    `json:"provider"`
	BaseURL         string    `json:"baseUrl"`
	ClientID        string    `json:"clientId"`
	Connected       bool      `json:"connected"`
	Sandbox         bool      `json:"sandbox"`
	ConnectedAt     time.Time `json:"connectedAt"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt"`
}

type BankTransactionEvent struct {
	EventID            string         `json:"eventId"`
	BankCustomerID     string         `json:"bankCustomerId"`
	BankAccountID      string         `json:"bankAccountId"`
	AmountPaise        int64          `json:"amountPaise"`
	Counterparty       string         `json:"counterparty"`
	Channel            string         `json:"channel"`
	EventTimestamp     time.Time      `json:"eventTimestamp"`
	DeviceTrustScore   float64        `json:"deviceTrustScore"`
	BehaviourDrift     float64        `json:"behaviourDrift"`
	FailedAuthAttempts int            `json:"failedAuthAttempts"`
	IsSynthetic        bool           `json:"isSynthetic"`
	Source             string         `json:"source"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type RealtimeDetectionResult struct {
	EventID        string                `json:"eventId"`
	UserID         string                `json:"userId"`
	BankAccountID  string                `json:"bankAccountId"`
	RiskLevel      transaction.RiskLevel `json:"riskLevel"`
	RiskScore      float64               `json:"riskScore"`
	Action         string                `json:"action"`
	RequiresStepUp bool                  `json:"requiresStepUp"`
	Reason         string                `json:"reason"`
	IsSynthetic    bool                  `json:"isSynthetic"`
	ModelVersion   string                `json:"modelVersion"`
	ProcessedAt    time.Time             `json:"processedAt"`
}

type SyntheticBankStreamRequest struct {
	Count         int       `json:"count"`
	Seed          int64     `json:"seed"`
	BaseTimestamp time.Time `json:"baseTimestamp"`
}

type SyntheticBankStreamResponse struct {
	GeneratedCount int                       `json:"generatedCount"`
	Events         []BankTransactionEvent    `json:"events"`
	Detections     []RealtimeDetectionResult `json:"detections"`
	Summary        map[string]any            `json:"summary"`
	GeneratedAt    time.Time                 `json:"generatedAt"`
}

func (s *Service) PredictBehaviour(userID string) (BehaviourPrediction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return BehaviourPrediction{}, fmt.Errorf("user not found")
	}

	history := s.transactions[userID]
	goals := s.goals[userID]
	accounts := s.accounts[userID]
	profile := s.authProfiles[userID]

	var highRiskCount, blockedCount, lateNightCount int
	for _, tx := range history {
		if tx.RiskLevel == transaction.RiskHigh {
			highRiskCount++
		}
		if tx.Status == "blocked" {
			blockedCount++
		}
		hour := tx.CreatedAt.Hour()
		if hour <= 5 || hour >= 23 {
			lateNightCount++
		}
	}

	txnCount := len(history)
	txnDen := float64(maxInt(txnCount, 1))
	highRiskRatio := float64(highRiskCount) / txnDen
	blockedRatio := float64(blockedCount) / txnDen
	lateNightRatio := float64(lateNightCount) / txnDen

	goalProgress := 0.5
	if len(goals) > 0 {
		var targetTotal, savedTotal int64
		for _, g := range goals {
			targetTotal += g.TargetAmountPaise
			savedTotal += g.SavedAmountPaise
		}
		if targetTotal > 0 {
			goalProgress = clamp(float64(savedTotal)/float64(targetTotal), 0, 1)
		}
	}

	var totalBalance int64
	for _, acct := range accounts {
		totalBalance += acct.BalancePaise
	}

	failedAuthRatio := 0.0
	if profile != nil {
		failedAuthRatio = clamp(float64(profile.FailedLoginCount)/10.0, 0, 1)
	}

	fraudProb := clamp(0.08+(highRiskRatio*0.35)+(blockedRatio*0.28)+(lateNightRatio*0.16)+(failedAuthRatio*0.13), 0.01, 0.99)
	securityRisk := clamp(fraudProb*100, 1, 99)
	stability := clamp((1.0-((highRiskRatio*0.5)+(blockedRatio*0.35)+(lateNightRatio*0.15)))*100, 5, 98)
	spendingDiscipline := clamp((1.0-(blockedRatio*0.55)-(highRiskRatio*0.25))*100, 5, 99)
	goalCompletionProb := clamp((goalProgress*0.6)+(stability/100.0*0.4), 0.05, 0.98)

	segment := "balanced_builder"
	switch {
	case securityRisk >= 70:
		segment = "security_sensitive"
	case goalCompletionProb >= 0.72 && spendingDiscipline >= 70:
		segment = "goal_accelerator"
	case totalBalance <= 4000000:
		segment = "liquidity_focused"
	}

	nudge := "Continue SIP automation and periodic goal top-ups."
	if securityRisk >= 70 {
		nudge = "Enable strict transaction limits and step-up authentication for all new beneficiaries."
	} else if goalCompletionProb < 0.55 {
		nudge = "Increase monthly SIP by 10-15% to avoid goal slippage."
	}

	features := map[string]float64{
		"txn_count":            float64(txnCount),
		"high_risk_txn_ratio":  highRiskRatio,
		"blocked_txn_ratio":    blockedRatio,
		"late_night_txn_ratio": lateNightRatio,
		"failed_auth_ratio":    failedAuthRatio,
		"goal_progress_ratio":  goalProgress,
		"total_balance_lakhs":  float64(totalBalance) / 10000000.0,
	}

	return BehaviourPrediction{
		UserID:                    user.ID,
		Segment:                   segment,
		SecurityRiskScore:         round2(securityRisk),
		FraudProbability:          round4(fraudProb),
		BehaviourStabilityScore:   round2(stability),
		SpendingDisciplineScore:   round2(spendingDiscipline),
		GoalCompletionProbability: round4(goalCompletionProb),
		RecommendedNudge:          nudge,
		FeatureVector:             features,
		GeneratedAt:               time.Now().UTC(),
	}, nil
}

func (s *Service) RecommendInvestments(userID string) (InvestmentRecommendation, error) {
	prediction, err := s.PredictBehaviour(userID)
	if err != nil {
		return InvestmentRecommendation{}, err
	}

	health, err := s.HealthScore(userID)
	if err != nil {
		return InvestmentRecommendation{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return InvestmentRecommendation{}, fmt.Errorf("user not found")
	}

	goals := append([]Goal(nil), s.goals[userID]...)
	kyc := s.kycProfiles[userID]

	monthlyIncome := int64(7000000)
	if kyc != nil {
		if annual := parseAnnualIncomePaise(kyc.AnnualIncome); annual > 0 {
			monthlyIncome = annual / 12
		}
	}

	expenseFactor := 0.68 + ((100.0-prediction.SpendingDisciplineScore)/100.0)*0.12
	monthlyExpense := int64(float64(monthlyIncome) * clamp(expenseFactor, 0.55, 0.88))
	investable := monthlyIncome - monthlyExpense
	if investable < 500000 {
		investable = 500000
	}

	riskProfile := "moderate"
	switch {
	case prediction.SecurityRiskScore >= 70 || strings.EqualFold(health.Band, "needs_attention"):
		riskProfile = "conservative"
	case health.Score300To900 >= 760 && prediction.BehaviourStabilityScore >= 70:
		riskProfile = "growth"
	}

	sipFactor := 0.55
	assetSplit := map[string]float64{"equity": 45, "debt": 35, "gold": 10, "liquid": 10}
	switch riskProfile {
	case "conservative":
		sipFactor = 0.40
		assetSplit = map[string]float64{"equity": 25, "debt": 50, "gold": 15, "liquid": 10}
	case "growth":
		sipFactor = 0.65
		assetSplit = map[string]float64{"equity": 65, "debt": 20, "gold": 10, "liquid": 5}
	}

	monthlySIP := int64(float64(investable) * sipFactor)
	emergencyFundTarget := monthlyExpense * 6

	now := time.Now().UTC()
	goalRecs := make([]GoalFundingRecommendation, 0, len(goals))
	for _, goal := range goals {
		if strings.EqualFold(goal.Status, "closed") {
			continue
		}
		remaining := goal.TargetAmountPaise - goal.SavedAmountPaise
		if remaining < 0 {
			remaining = 0
		}
		monthsLeft := int(math.Ceil(goal.TargetDate.Sub(now).Hours() / (24 * 30)))
		if monthsLeft < 1 {
			monthsLeft = 1
		}
		requiredMonthly := int64(math.Ceil(float64(maxInt64(remaining, 1)) / float64(monthsLeft)))
		priority := "normal"
		if monthsLeft <= 24 {
			priority = "high"
		} else if monthsLeft <= 48 {
			priority = "medium"
		}
		coverageRatio := clamp(float64(monthlySIP)/float64(maxInt64(requiredMonthly, 1)), 0, 2)
		projectedProb := clamp(prediction.GoalCompletionProbability+(coverageRatio-1.0)*0.22, 0.05, 0.98)

		goalRecs = append(goalRecs, GoalFundingRecommendation{
			GoalID:                           goal.ID,
			GoalName:                         goal.Name,
			RemainingAmountPaise:             remaining,
			RequiredMonthlyContributionPaise: requiredMonthly,
			MonthsLeft:                       monthsLeft,
			Priority:                         priority,
			ProjectedCompletionProbability:   round4(projectedProb),
		})
	}

	rebalance := []string{
		"Use account-aggregator refresh weekly to keep cashflow and risk features current.",
		"Auto-adjust SIP by +/-5% whenever behaviour stability moves by more than 10 points.",
	}
	if prediction.SecurityRiskScore >= 70 {
		rebalance = append(rebalance, "Temporarily cap equity SIP share until security risk score drops below 60.")
	}

	explain := []string{
		fmt.Sprintf("Risk profile set to %s using health score %d and security risk %.2f.", riskProfile, health.Score300To900, prediction.SecurityRiskScore),
		fmt.Sprintf("Monthly SIP uses %.0f%% of investable cashflow with an emergency reserve target of 6 months.", sipFactor*100),
		"Goal funding suggestions are constrained by remaining horizon and target amount.",
	}

	return InvestmentRecommendation{
		UserID:                   userID,
		RiskProfile:              riskProfile,
		MonthlySIPPaise:          monthlySIP,
		EmergencyFundTargetPaise: emergencyFundTarget,
		SuggestedAssetSplit:      assetSplit,
		GoalRecommendations:      goalRecs,
		RebalanceActions:         rebalance,
		Explainability:           explain,
		GeneratedAt:              now,
	}, nil
}

func (s *Service) GenerateSyntheticDataset(spec SyntheticDatasetSpec) SyntheticDataset {
	if spec.UserCount <= 0 {
		spec.UserCount = 1000
	}
	if spec.Days <= 0 {
		spec.Days = 180
	}
	if spec.Seed == 0 {
		spec.Seed = time.Now().UnixNano()
	}

	rng := mrand.New(mrand.NewSource(spec.Seed))
	rows := make([]SyntheticBehaviourRow, 0, spec.UserCount)

	fraudCount := 0
	totalFraudProb := 0.0
	totalSavingsRate := 0.0
	totalSIPAffinity := 0.0

	for i := 0; i < spec.UserCount; i++ {
		monthlyIncome := int64(30000+rng.Intn(220000)) * 100
		expenseRatio := clamp(0.45+rng.Float64()*0.45, 0.35, 0.95)
		monthlyExpense := int64(float64(monthlyIncome) * expenseRatio)
		savingsRate := clamp(float64(monthlyIncome-monthlyExpense)/float64(maxInt64(monthlyIncome, 1)), 0, 0.8)

		txnCount := 30 + rng.Intn(250)
		lateNight := clamp(rng.Float64()*0.4, 0, 1)
		newBeneficiary := clamp(rng.Float64()*0.35, 0, 1)
		failedAuth := rng.Intn(6)
		behaviourDrift := clamp(0.1+rng.Float64()*0.8, 0, 1)

		fraudProb := clamp(0.08+(lateNight*0.25)+(newBeneficiary*0.30)+(float64(failedAuth)*0.07)+(behaviourDrift*0.30)+(rng.Float64()*0.1), 0.01, 0.99)
		fraudLabel := fraudProb >= 0.65
		if fraudLabel {
			fraudCount++
		}

		sipAffinity := clamp(0.2+(savingsRate*0.6)+((1-fraudProb)*0.2), 0, 1)

		row := SyntheticBehaviourRow{
			SyntheticUserID:           fmt.Sprintf("syn-user-%05d", i+1),
			MonthlyIncomePaise:        monthlyIncome,
			MonthlyExpensePaise:       monthlyExpense,
			SavingsRate:               round4(savingsRate),
			TransactionCount:          txnCount,
			LateNightTransactionRatio: round4(lateNight),
			NewBeneficiaryRatio:       round4(newBeneficiary),
			FailedAuthAttempts:        failedAuth,
			BehaviourDriftScore:       round4(behaviourDrift),
			FraudRiskProbability:      round4(fraudProb),
			FraudLabel:                fraudLabel,
			SIPAffinityScore:          round4(sipAffinity),
		}
		rows = append(rows, row)
		totalFraudProb += fraudProb
		totalSavingsRate += savingsRate
		totalSIPAffinity += sipAffinity
	}

	summary := map[string]any{
		"user_count":             spec.UserCount,
		"days":                   spec.Days,
		"seed":                   spec.Seed,
		"avg_fraud_probability":  round4(totalFraudProb / float64(maxInt(spec.UserCount, 1))),
		"high_risk_user_count":   fraudCount,
		"avg_savings_rate":       round4(totalSavingsRate / float64(maxInt(spec.UserCount, 1))),
		"avg_sip_affinity_score": round4(totalSIPAffinity / float64(maxInt(spec.UserCount, 1))),
	}

	return SyntheticDataset{
		Spec:        spec,
		Rows:        rows,
		Summary:     summary,
		GeneratedAt: time.Now().UTC(),
	}
}

func (s *Service) BankAPIReadiness() BankAPIReadiness {
	return BankAPIReadiness{
		Status:          "ready_for_adapter_integration",
		ContractVersion: "v1",
		OpenStandards: []string{
			"Open Banking / FAPI",
			"Account Aggregator (consent artifact and FIU/FIP flows)",
			"ISO 20022 compatible transaction semantics",
		},
		AuthModes: []string{
			"OAuth 2.1 with PKCE",
			"mTLS for server-to-server calls",
			"JWT client assertions",
		},
		RequiredEndpoints: []string{
			"GET /bank/accounts",
			"GET /bank/accounts/{id}/balances",
			"GET /bank/accounts/{id}/transactions",
			"POST /bank/beneficiaries",
			"POST /bank/payments/initiate",
			"GET /bank/payments/{id}/status",
			"POST /bank/consents",
			"POST /bank/webhooks/transactions",
			"POST /bank/webhooks/payments",
		},
		WebhookEvents: []string{
			"payment.status.updated",
			"beneficiary.status.updated",
			"consent.revoked",
			"account.balance.updated",
		},
		FieldMapping: map[string]string{
			"bank_customer_id":        "internalUserId",
			"bank_account_id":         "account.id",
			"bank_transaction_status": "transaction.status",
			"bank_risk_signal":        "aiml.securityRiskScore",
			"bank_goal_tag":           "goal.name",
		},
		IntegrationNotes: []string{
			"Keep all bank integrations behind connector adapters to avoid domain model drift.",
			"Use idempotency keys and webhook replay protection for payment-state updates.",
			"Persist model inputs/features per inference for audit and explainability.",
		},
	}
}

func (s *Service) ConnectBankAPI(userID string, req BankConnectionRequest) (BankConnectionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return BankConnectionStatus{}, fmt.Errorf("user not found")
	}

	now := time.Now().UTC()

	var status BankConnectionStatus
	// Establish the connection through the bank provider when one is injected;
	// otherwise use the legacy in-process sandbox behaviour.
	if bank := s.bankAdapter(); bank != nil {
		st, err := bank.Connect(context.Background(), userID, req)
		if err != nil {
			return BankConnectionStatus{}, err
		}
		status = st
	} else {
		status = BankConnectionStatus{
			ConnectionID:    "bankconn_" + randomHex(8),
			Provider:        firstNonEmpty(strings.TrimSpace(req.Provider), "sandbox-bank"),
			BaseURL:         firstNonEmpty(strings.TrimSpace(req.BaseURL), "https://sandbox-bank.local"),
			ClientID:        firstNonEmpty(strings.TrimSpace(req.ClientID), "FINIX-client"),
			Connected:       true,
			Sandbox:         req.Sandbox,
			ConnectedAt:     now,
			LastHeartbeatAt: now,
		}
		if existing, ok := s.bankConnections[userID]; ok {
			status.ConnectionID = existing.ConnectionID
			status.ConnectedAt = existing.ConnectedAt
		}
	}

	s.bankConnections[userID] = status
	s.appendAuditLocked(userID, "bank_api_connected", "bank_api", "success", "Bank API connector configured", "System")
	return status, nil
}

func (s *Service) BankConnection(userID string) (BankConnectionStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return BankConnectionStatus{}, fmt.Errorf("user not found")
	}
	status, ok := s.bankConnections[userID]
	if !ok {
		return BankConnectionStatus{}, fmt.Errorf("bank api is not connected")
	}
	return status, nil
}

func (s *Service) IngestBankEvent(userID string, event BankTransactionEvent) (RealtimeDetectionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return RealtimeDetectionResult{}, fmt.Errorf("user not found")
	}

	if connection, ok := s.bankConnections[userID]; !ok || !connection.Connected {
		return RealtimeDetectionResult{}, fmt.Errorf("bank api is not connected")
	}

	if customer := strings.TrimSpace(event.BankCustomerID); customer != "" && customer != userID {
		return RealtimeDetectionResult{}, fmt.Errorf("bankCustomerId does not match authenticated user")
	}

	if event.AmountPaise <= 0 {
		return RealtimeDetectionResult{}, fmt.Errorf("amountPaise must be greater than zero")
	}
	if strings.TrimSpace(event.Counterparty) == "" {
		return RealtimeDetectionResult{}, fmt.Errorf("counterparty is required")
	}

	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = "bankevt_" + randomHex(8)
	}
	if event.EventTimestamp.IsZero() {
		event.EventTimestamp = time.Now().UTC()
	}
	event.Channel = firstNonEmpty(strings.TrimSpace(event.Channel), "bank_transfer")
	event.Source = firstNonEmpty(strings.TrimSpace(event.Source), "bank_api")
	event.BankCustomerID = firstNonEmpty(strings.TrimSpace(event.BankCustomerID), userID)

	history := s.transactions[userID]
	// Floor the denominator at FLOOR_AVG (spec §1.1) — see InitiateTransaction.
	avgAmount := historicalAverageAmount(history)
	if floor := float64(s.params.AmountTerm.FloorAvgPaise); avgAmount < floor {
		avgAmount = floor
	}

	_, seenRecipient := s.beneficiaries[userID][strings.ToLower(event.Counterparty)]
	signal := transaction.RiskSignal{
		AmountVsAverage:   float64(event.AmountPaise) / avgAmount,
		IsNewRecipient:    !seenRecipient,
		HourOfDay:         event.EventTimestamp.Hour(),
		FailedPINAttempts: event.FailedAuthAttempts,
		VelocityCount1Hr:  velocityLastHour(history, event.EventTimestamp),
		BalanceImpact:     balanceImpact(history, event.AmountPaise, s.accounts[userID]),
		RecipientGNNScore: s.fraudGraph.GetRiskScore(event.Counterparty),
		BehaviourDrift:    clamp(event.BehaviourDrift, 0, 1),
		SessionTrustScore: clamp(event.DeviceTrustScore, 0, 1),
	}

	assessment := s.riskEngine.Evaluate(signal)
	action := "allow"
	status := "success"
	requiresStepUp := false
	if assessment.Level == transaction.RiskMedium {
		action = "step_up"
		status = "warning_ack_required"
		requiresStepUp = true
	}
	if assessment.Level == transaction.RiskHigh {
		action = "block"
		status = "blocked"
		requiresStepUp = true
	}

	tx := Transaction{
		ID:             "banktxn_" + randomHex(8),
		UserID:         userID,
		AmountPaise:    event.AmountPaise,
		Currency:       "INR",
		DebitCredit:    "debit",
		Recipient:      event.Counterparty,
		MerchantName:   event.Counterparty,
		Channel:        event.Channel,
		Description:    fmt.Sprintf("Bank transaction to %s of INR %.2f via %s", event.Counterparty, float64(event.AmountPaise)/100, event.Channel),
		Status:         status,
		RiskLevel:      assessment.Level,
		RiskScore:      assessment.Score,
		XAIReason:      assessment.Reason,
		CreatedAt:      event.EventTimestamp,
		IdempotencyKey: event.EventID,
		TimelineIDs:    []string{},
		LinkedAccount:  firstNonEmpty(strings.TrimSpace(event.BankAccountID), ""),
	}
	s.transactions[userID] = append(s.transactions[userID], tx)
	s.fraudGraph.AddTransaction(userID, event.Counterparty, event.AmountPaise)

	if status == "success" {
		acct := s.accounts[userID]
		if len(acct) > 0 {
			narration := fmt.Sprintf("Bank tx to %s — INR %.2f via %s", event.Counterparty, float64(event.AmountPaise)/100, event.Channel)
			s.accountingLedger.PostOutgoingPayment(event.EventID, acct[0].ID, userID, event.Counterparty, event.AmountPaise, narration)
		}
		// C-5 fix: verify funds before debiting. On insufficient funds, reverse the
		// recorded status to "blocked" so no phantom money is created.
		if err := s.applyDebitToFirstAccountLocked(userID, event.AmountPaise); err != nil {
			status = "blocked"
			if n := len(s.transactions[userID]); n > 0 {
				s.transactions[userID][n-1].Status = "blocked"
				s.transactions[userID][n-1].XAIReason = "Blocked: insufficient funds for debit."
			}
		} else {
			s.beneficiaries[userID][strings.ToLower(event.Counterparty)] = struct{}{}
		}
	}

	detection := RealtimeDetectionResult{
		EventID:        event.EventID,
		UserID:         userID,
		BankAccountID:  firstNonEmpty(strings.TrimSpace(event.BankAccountID), "unknown"),
		RiskLevel:      assessment.Level,
		RiskScore:      round2(assessment.Score),
		Action:         action,
		RequiresStepUp: requiresStepUp,
		Reason:         assessment.Reason,
		IsSynthetic:    event.IsSynthetic,
		ModelVersion:   "risk-engine-v1",
		ProcessedAt:    time.Now().UTC(),
	}

	existing := s.realtimeDetections[userID]
	updated := append([]RealtimeDetectionResult{detection}, existing...)
	if len(updated) > 500 {
		updated = updated[:500]
	}
	s.realtimeDetections[userID] = updated

	conn := s.bankConnections[userID]
	conn.LastHeartbeatAt = time.Now().UTC()
	s.bankConnections[userID] = conn

	s.appendAuditLocked(userID, "bank_event_detected", "bank_api", action, assessment.Reason, "AI")
	return detection, nil
}

func (s *Service) RealtimeDetections(userID string, limit int) ([]RealtimeDetectionResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return nil, fmt.Errorf("user not found")
	}
	out := append([]RealtimeDetectionResult(nil), s.realtimeDetections[userID]...)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Service) StreamSyntheticBankEvents(userID string, req SyntheticBankStreamRequest) (SyntheticBankStreamResponse, error) {
	if req.Count <= 0 {
		req.Count = 25
	}
	if req.Count > 500 {
		req.Count = 500
	}
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}
	if req.BaseTimestamp.IsZero() {
		req.BaseTimestamp = time.Now().UTC().Add(-1 * time.Hour)
	}

	if _, err := s.BankConnection(userID); err != nil {
		_, err = s.ConnectBankAPI(userID, BankConnectionRequest{
			Provider: "synthetic-bank",
			BaseURL:  "https://synthetic-bank.local",
			ClientID: "synthetic-client",
			Sandbox:  true,
		})
		if err != nil {
			return SyntheticBankStreamResponse{}, err
		}
	}

	rng := mrand.New(mrand.NewSource(req.Seed))
	events := make([]BankTransactionEvent, 0, req.Count)
	detections := make([]RealtimeDetectionResult, 0, req.Count)
	actionCount := map[string]int{"allow": 0, "step_up": 0, "block": 0}

	for i := 0; i < req.Count; i++ {
		attackLike := rng.Float64() < 0.16
		amountPaise := int64(5000+rng.Intn(900000)) * 100
		counterparty := fmt.Sprintf("known-beneficiary-%02d", rng.Intn(30)+1)
		trust := clamp(0.70+rng.Float64()*0.25, 0, 1)
		drift := clamp(0.08+rng.Float64()*0.25, 0, 1)
		failedAuth := rng.Intn(2)

		if attackLike {
			amountPaise = int64(500000+rng.Intn(5000000)) * 100
			counterparty = fmt.Sprintf("unknown-urgent-%02d", rng.Intn(90)+10)
			trust = clamp(0.10+rng.Float64()*0.25, 0, 1)
			drift = clamp(0.65+rng.Float64()*0.30, 0, 1)
			failedAuth = 2 + rng.Intn(3)
		}

		eventTime := req.BaseTimestamp.Add(time.Duration((i*3)+rng.Intn(3)) * time.Minute)
		event := BankTransactionEvent{
			EventID:            fmt.Sprintf("synbankevt_%05d", i+1),
			BankCustomerID:     userID,
			BankAccountID:      fmt.Sprintf("bankacct_%03d", rng.Intn(5)+1),
			AmountPaise:        amountPaise,
			Counterparty:       counterparty,
			Channel:            "bank_transfer",
			EventTimestamp:     eventTime,
			DeviceTrustScore:   trust,
			BehaviourDrift:     drift,
			FailedAuthAttempts: failedAuth,
			IsSynthetic:        true,
			Source:             "synthetic_bank_api",
			Metadata:           map[string]any{"attack_like": attackLike},
		}

		result, err := s.IngestBankEvent(userID, event)
		if err != nil {
			return SyntheticBankStreamResponse{}, err
		}

		events = append(events, event)
		detections = append(detections, result)
		actionCount[result.Action]++
	}

	summary := map[string]any{
		"seed":            req.Seed,
		"generated_count": req.Count,
		"allow_count":     actionCount["allow"],
		"step_up_count":   actionCount["step_up"],
		"block_count":     actionCount["block"],
	}

	return SyntheticBankStreamResponse{
		GeneratedCount: req.Count,
		Events:         events,
		Detections:     detections,
		Summary:        summary,
		GeneratedAt:    time.Now().UTC(),
	}, nil
}

func parseAnnualIncomePaise(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	onlyDigits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, trimmed)
	if onlyDigits == "" {
		return 0
	}
	v, err := strconv.ParseInt(onlyDigits, 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v * 100
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
