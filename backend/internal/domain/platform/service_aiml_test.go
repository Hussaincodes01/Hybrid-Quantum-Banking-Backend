package platform

import (
	"testing"
	"time"
)

func TestPredictBehaviourAndInvestmentRecommendations(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Ishita",
		Mobile:              "7777777777",
		Email:               "ishita@example.com",
		DeviceIDFingerprint: "dev-777",
		DeviceType:          "android",
		AppVersion:          "1.0.0",
		IPAddress:           "10.20.30.40",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = svc.CreateGoal(reg.UserID, GoalCreateRequest{
		Name:              "House Down Payment",
		Description:       "Save for a house down payment",
		TargetAmountPaise: 250000000,
		Currency:          "INR",
		Frequency:         "monthly",
		Priority:          "high",
		TargetDate:        time.Now().AddDate(3, 0, 0),
	})
	if err != nil {
		t.Fatalf("create goal failed: %v", err)
	}

	_, err = svc.InitiateTransaction(reg.UserID, "idem-aiml-1", InitiateTransactionRequest{
		AmountPaise:       250000,
		Recipient:         "known friend",
		Channel:           "upi",
		SessionTrustScore: 0.9,
		BehaviourDrift:    0.1,
	})
	if err != nil {
		t.Fatalf("initiate low-risk transaction failed: %v", err)
	}

	_, err = svc.InitiateTransaction(reg.UserID, "idem-aiml-2", InitiateTransactionRequest{
		AmountPaise:       7000000,
		Recipient:         "unknown urgent",
		Channel:           "bank_transfer",
		SessionTrustScore: 0.2,
		BehaviourDrift:    0.8,
		FailedPINAttempts: 3,
	})
	if err != nil {
		t.Fatalf("initiate high-risk transaction failed: %v", err)
	}

	prediction, err := svc.PredictBehaviour(reg.UserID)
	if err != nil {
		t.Fatalf("predict behaviour failed: %v", err)
	}
	if prediction.Segment == "" {
		t.Fatal("expected non-empty segment")
	}
	if prediction.SecurityRiskScore <= 0 || prediction.SecurityRiskScore > 100 {
		t.Fatalf("security risk score out of range: %.2f", prediction.SecurityRiskScore)
	}
	if _, ok := prediction.FeatureVector["blocked_txn_ratio"]; !ok {
		t.Fatal("expected blocked_txn_ratio feature in output")
	}

	rec, err := svc.RecommendInvestments(reg.UserID)
	if err != nil {
		t.Fatalf("recommend investments failed: %v", err)
	}
	if rec.MonthlySIPPaise <= 0 {
		t.Fatal("expected positive monthly SIP recommendation")
	}
	if len(rec.SuggestedAssetSplit) == 0 {
		t.Fatal("expected non-empty asset split")
	}
	if len(rec.Explainability) == 0 {
		t.Fatal("expected explainability reasons")
	}
}

func TestSyntheticDatasetGeneration(t *testing.T) {
	svc := NewService()
	dataset := svc.GenerateSyntheticDataset(SyntheticDatasetSpec{UserCount: 50, Days: 120, Seed: 42})

	if len(dataset.Rows) != 50 {
		t.Fatalf("expected 50 synthetic rows, got %d", len(dataset.Rows))
	}
	if dataset.Spec.Seed != 42 {
		t.Fatalf("expected seed to remain 42, got %d", dataset.Spec.Seed)
	}
	if dataset.Rows[0].SyntheticUserID == "" {
		t.Fatal("expected synthetic user id in first row")
	}
	if dataset.Rows[0].FraudRiskProbability < 0 || dataset.Rows[0].FraudRiskProbability > 1 {
		t.Fatalf("fraud probability out of range: %.4f", dataset.Rows[0].FraudRiskProbability)
	}
	if _, ok := dataset.Summary["avg_fraud_probability"]; !ok {
		t.Fatal("expected avg_fraud_probability in summary")
	}
}

func TestBankAPIReadinessContract(t *testing.T) {
	svc := NewService()
	readiness := svc.BankAPIReadiness()

	if readiness.Status == "" {
		t.Fatal("expected readiness status")
	}
	if len(readiness.RequiredEndpoints) == 0 {
		t.Fatal("expected required endpoints for bank integration")
	}
	if len(readiness.AuthModes) == 0 {
		t.Fatal("expected auth modes")
	}
	if readiness.FieldMapping["bank_account_id"] == "" {
		t.Fatal("expected field mapping for bank_account_id")
	}
}

func TestRealtimeBankAPIIngestionAndSyntheticStream(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Neha",
		Mobile:              "7666666666",
		Email:               "neha@example.com",
		DeviceIDFingerprint: "dev-neha",
		DeviceType:          "android",
		AppVersion:          "1.3.0",
		IPAddress:           "10.1.1.15",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	connection, err := svc.ConnectBankAPI(reg.UserID, BankConnectionRequest{
		Provider: "fiu-sandbox",
		BaseURL:  "https://sandbox-bank.example",
		ClientID: "FINIX-e2e",
		Sandbox:  true,
	})
	if err != nil {
		t.Fatalf("connect bank api failed: %v", err)
	}
	if !connection.Connected {
		t.Fatal("expected connected bank status")
	}

	detection, err := svc.IngestBankEvent(reg.UserID, BankTransactionEvent{
		EventID:            "evt-manual-1",
		BankCustomerID:     reg.UserID,
		BankAccountID:      "bankacct-001",
		AmountPaise:        9000000,
		Counterparty:       "unknown urgent recipient",
		Channel:            "bank_transfer",
		EventTimestamp:     time.Now().UTC(),
		DeviceTrustScore:   0.15,
		BehaviourDrift:     0.85,
		FailedAuthAttempts: 3,
		IsSynthetic:        false,
		Source:             "bank_api",
	})
	if err != nil {
		t.Fatalf("ingest bank event failed: %v", err)
	}
	if detection.EventID == "" {
		t.Fatal("expected detection event id")
	}
	if detection.Action == "" {
		t.Fatal("expected detection action")
	}

	history, err := svc.RealtimeDetections(reg.UserID, 10)
	if err != nil {
		t.Fatalf("realtime detections failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected non-empty detection history")
	}

	stream, err := svc.StreamSyntheticBankEvents(reg.UserID, SyntheticBankStreamRequest{
		Count: 12,
		Seed:  20260412,
	})
	if err != nil {
		t.Fatalf("synthetic stream failed: %v", err)
	}
	if stream.GeneratedCount != 12 {
		t.Fatalf("expected 12 generated events, got %d", stream.GeneratedCount)
	}
	if len(stream.Events) != 12 || len(stream.Detections) != 12 {
		t.Fatalf("expected 12 events and detections, got %d and %d", len(stream.Events), len(stream.Detections))
	}
	if !stream.Events[0].IsSynthetic {
		t.Fatal("expected synthetic flag on generated bank event")
	}
	if _, ok := stream.Summary["block_count"]; !ok {
		t.Fatal("expected block_count in stream summary")
	}
}
