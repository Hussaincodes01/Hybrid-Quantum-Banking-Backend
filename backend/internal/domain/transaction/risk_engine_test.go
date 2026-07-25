package transaction

import "testing"

func TestRiskEngineLowRisk(t *testing.T) {
	engine := NewRiskEngine()
	assessment := engine.Evaluate(RiskSignal{
		AmountVsAverage:   1.1,
		IsNewRecipient:    false,
		HourOfDay:         14,
		FailedPINAttempts: 0,
		VelocityCount1Hr:  1,
		BalanceImpact:     0.1,
		RecipientGNNScore: 0.0,
		BehaviourDrift:    0.1,
		SessionTrustScore: 0.95,
	})

	if assessment.Level != RiskLow {
		t.Fatalf("expected low risk, got %s", assessment.Level)
	}
}

func TestRiskEngineHighRisk(t *testing.T) {
	engine := NewRiskEngine()
	assessment := engine.Evaluate(RiskSignal{
		AmountVsAverage:   12,
		IsNewRecipient:    true,
		HourOfDay:         1,
		FailedPINAttempts: 3,
		VelocityCount1Hr:  7,
		BalanceImpact:     0.92,
		RecipientGNNScore: 0.9,
		BehaviourDrift:    0.8,
		SessionTrustScore: 0.2,
	})

	if assessment.Level != RiskHigh {
		t.Fatalf("expected high risk, got %s (score %.2f)", assessment.Level, assessment.Score)
	}
}
