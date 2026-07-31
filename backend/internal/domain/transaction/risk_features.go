package transaction

// featureMaps projects a RiskSignal onto the transaction_risk_model.onnx feature
// names (the notebook FEATURE_COLUMNS / MODEL_IO_CONTRACT.md). It returns a
// numeric map and a categorical map that preprocess.Artifact.BuildVector then
// label-encodes and StandardScales into the ordered 32-float vector.
//
// Convention for graceful degradation: keys omitted from the maps are filled by
// BuildVector with the training mean (scaled to 0). We therefore:
//   - always send the core signals the caller reliably computes,
//   - always send the boolean posture flags (false == a real 0), and
//   - send the continuous magnitudes / categoricals only when populated, so
//     backends that do not yet collect them fall back to the neutral mean
//     rather than an out-of-distribution raw 0.
//
// Derived features mirror the notebook exactly:
//   velocity_term         = min(transactions_last_1h * 2, 10)
//   recipient_seen_before = 1 - is_new_recipient
func (s RiskSignal) featureMaps() (numeric map[string]float64, categorical map[string]string) {
	numeric = map[string]float64{
		// Core signals: always provided by the service layer.
		"amount_vs_average":    s.AmountVsAverage,
		"velocity_term":        min(float64(s.VelocityCount1Hr)*2, 10),
		"balance_impact":       s.BalanceImpact,
		"transactions_last_1h": float64(s.VelocityCount1Hr),
		"failed_pin_attempts":  float64(s.FailedPINAttempts),
		"session_trust_score":  s.SessionTrustScore,
		"recipient_gnn_score":  s.RecipientGNNScore,
		"recipient_seen_before": boolF(!s.IsNewRecipient),

		// Posture flags: false is a meaningful 0, so always send.
		"registered_device_match": boolF(s.RegisteredDeviceMatch),
		"rooted_device":           boolF(s.RootedDevice),
		"emulator_detected":       boolF(s.EmulatorDetected),
		"otp_verified":            boolF(s.OTPVerified),
		"biometric_verified":      boolF(s.BiometricVerified),
		"recipient_verified":      boolF(s.RecipientVerified),
		"geo_velocity_flag":       boolF(s.GeoVelocityFlag),
		"structuring_flag":        boolF(s.StructuringFlag),
	}

	// Continuous magnitudes: send only when populated (>0), else mean-default.
	putIfPos(numeric, "transaction_amount", s.TransactionAmount)
	putIfPos(numeric, "transactions_last_24h", float64(s.TransactionsLast24h))
	putIfPos(numeric, "account_age_days", float64(s.AccountAgeDays))
	putIfPos(numeric, "current_balance", s.CurrentBalance)
	putIfPos(numeric, "monthly_income", s.MonthlyIncome)
	putIfPos(numeric, "recipient_account_age_days", float64(s.RecipientAccountAgeDays))
	putIfPos(numeric, "market_volatility_index", s.MarketVolatilityIndex)

	categorical = map[string]string{}
	putIfSet(categorical, "transaction_type", s.TransactionType)
	putIfSet(categorical, "payment_channel", s.PaymentChannel)
	putIfSet(categorical, "merchant_category", s.MerchantCategory)
	putIfSet(categorical, "currency", s.Currency)
	putIfSet(categorical, "account_type", s.AccountType)
	putIfSet(categorical, "occupation", s.Occupation)
	putIfSet(categorical, "kyc_status", s.KYCStatus)
	putIfSet(categorical, "device_type", s.DeviceType)
	putIfSet(categorical, "authentication_method", s.AuthenticationMethod)

	return numeric, categorical
}

func boolF(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func putIfPos(m map[string]float64, key string, v float64) {
	if v > 0 {
		m[key] = v
	}
}

func putIfSet(m map[string]string, key, v string) {
	if v != "" {
		m[key] = v
	}
}
