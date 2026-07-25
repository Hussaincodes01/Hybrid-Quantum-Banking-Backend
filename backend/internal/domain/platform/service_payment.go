package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"FINIX/backend/internal/domain/blockchain"
	"FINIX/backend/internal/domain/security"
)

type InitiatePaymentRequest struct {
	BeneficiaryID      string `json:"beneficiaryId"`
	BeneficiaryName    string `json:"beneficiaryName"`
	BeneficiaryAccount string `json:"beneficiaryAccount"`
	AmountPaise        int64  `json:"amountPaise"`
	Currency           string `json:"currency"`
	Method             string `json:"method"`
	OTPVerified        bool   `json:"otpVerified"`
	UserConsent        bool   `json:"userConsent"`
	QRCodePayload      string `json:"qrCodePayload,omitempty"`
	PaymentIntentID    string `json:"paymentIntentId,omitempty"`
	UPIIntentID        string `json:"upiIntentId,omitempty"`
	SIPID              string `json:"sipId,omitempty"`
}

type VerifyReceiptRequest struct {
	ReceiptHash  string `json:"receiptHash"`
	VerifiedBy   string `json:"verifiedBy"`
	VerifierType string `json:"verifierType"`
}

func (s *Service) InitiatePayment(userID string, req InitiatePaymentRequest) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return Payment{}, errors.New("user not found")
	}
	if req.AmountPaise <= 0 {
		return Payment{}, errors.New("amount must be greater than zero")
	}
	if strings.TrimSpace(req.BeneficiaryName) == "" && strings.TrimSpace(req.BeneficiaryAccount) == "" {
		return Payment{}, errors.New("beneficiary name or account is required")
	}

	// Bank pre-checks via the adapter (mock by default) BEFORE risk scoring:
	//   - insufficient balance is a hard error (the payer can't afford it);
	//   - an unresolvable beneficiary is flagged (raises risk), not hard-blocked,
	//     so a first-time/off-network payee still goes through the risk engine.
	beneficiaryUnverified := false
	if bank := s.bankAdapter(); bank != nil {
		payee := strings.TrimSpace(firstNonEmpty(req.BeneficiaryAccount, req.BeneficiaryName))
		if strings.Contains(payee, "@") {
			if _, _, ok, err := bank.VerifyUPI(context.Background(), payee); err == nil && !ok {
				beneficiaryUnverified = true
			}
		}
		// Best-effort balance check against the payer's accounts.
		for _, acct := range s.accounts[userID] {
			if bal, err := bank.GetBalance(context.Background(), acct.UPIID); err == nil {
				if bal < req.AmountPaise {
					return Payment{}, errors.New("insufficient balance for this payment")
				}
				break
			}
		}
	}

	now := time.Now().UTC()
	id := s.nextID("pay")
	currency := firstNonEmpty(req.Currency, "INR")
	method := firstNonEmpty(req.Method, "upi")
	status := "initiated"

	// Risk assessment on the payment
	riskScore := s.gnnScoreForRecipient(req.BeneficiaryName)
	if beneficiaryUnverified {
		// Unverified payee: elevate risk so the payment is flagged for step-up.
		if riskScore < 0.55 {
			riskScore = 0.55
		}
	}
	riskLevel := "low"
	if riskScore > 0.8 {
		riskLevel = "high"
	} else if riskScore > 0.5 {
		riskLevel = "medium"
	}

	// Sign with Dilithium
	var dilithiumSig *security.DilithiumSignature
	if s.dilithium != nil {
		payData := []byte(fmt.Sprintf("%s:%s:%s:%d:%s", id, userID, req.BeneficiaryName, req.AmountPaise, now.Format(time.RFC3339Nano)))
		sig, sigErr := s.dilithium.Sign(payData)
		if sigErr == nil {
			dilithiumSig = sig
		}
	}

	payment := Payment{
		ID:                 id,
		UserID:             userID,
		BeneficiaryID:      req.BeneficiaryID,
		BeneficiaryName:    req.BeneficiaryName,
		BeneficiaryAccount: req.BeneficiaryAccount,
		AmountPaise:        req.AmountPaise,
		Currency:           currency,
		Method:             method,
		Status:             status,
		RiskLevel:          riskLevel,
		RiskScore:          riskScore,
		StartedAt:          now,
		OTPVerified:        req.OTPVerified,
		UserConsent:        req.UserConsent,
		QRCodePayload:      req.QRCodePayload,
		PaymentIntentID:    req.PaymentIntentID,
		UPIIntentID:        req.UPIIntentID,
		SIPID:              req.SIPID,
		DilithiumSignature: dilithiumSig,
		CreatedAt:          now,
	}

	s.payments[userID] = append(s.payments[userID], payment)
	s.appendAuditLocked(userID, "payment_initiated", "payment", "initiated",
		fmt.Sprintf("Payment %s for INR %.2f via %s", id, float64(req.AmountPaise)/100, method), "User")
	// H-8: enforce smart-contract rules on the payment. A double-spend (same
	// payload replayed) or an active cooling-off period rejects the payment
	// instead of merely annotating it. The event is still recorded for audit.
	if _, err := s.ledger.WriteEventStrict(context.Background(), userID, "payment_initiated", "payment", payment); err != nil {
		if errors.Is(err, blockchain.ErrSmartContractViolation) {
			if pays := s.payments[userID]; len(pays) > 0 {
				pays[len(pays)-1].Status = "blocked"
			}
			s.appendAuditLocked(userID, "payment_initiated", "payment", "blocked",
				"Blocked by blockchain smart contract: "+err.Error(), "SmartContract")
		}
		return Payment{}, err
	}

	if s.db != nil {
		savedPayment := payment
		db := s.db
		go func() {
			_ = db.SavePaymentToDB(context.Background(), userID, savedPayment)
		}()
	}

	return payment, nil
}

func (s *Service) ListPayments(userID string) ([]Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return append([]Payment(nil), s.payments[userID]...), nil
}

func (s *Service) GetPayment(userID, paymentID string) (Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return Payment{}, errors.New("user not found")
	}
	for _, p := range s.payments[userID] {
		if p.ID == paymentID {
			return p, nil
		}
	}
	return Payment{}, errors.New("payment not found")
}

func (s *Service) VerifyPaymentReceipt(userID, paymentID string, req VerifyReceiptRequest) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return Payment{}, errors.New("user not found")
	}

	payments := s.payments[userID]
	for i := range payments {
		if payments[i].ID != paymentID {
			continue
		}
		if payments[i].ReceiptVerification != nil && payments[i].ReceiptVerification.Verified {
			return Payment{}, errors.New("payment receipt already verified")
		}

		now := time.Now().UTC()
		payments[i].ReceiptVerification = &ReceiptVerification{
			Verified:     true,
			VerifiedAt:   &now,
			VerifiedBy:   req.VerifiedBy,
			VerifierType: firstNonEmpty(req.VerifierType, "user"),
			ReceiptHash:  req.ReceiptHash,
		}
		payments[i].Status = "completed"
		payments[i].CompletedAt = &now

		s.payments[userID] = payments
		s.appendAuditLocked(userID, "payment_receipt_verified", "payment", "completed",
			fmt.Sprintf("Payment %s receipt verified by %s", paymentID, req.VerifiedBy), "User")
		_, _ = s.ledger.WriteEvent(context.Background(), userID, "payment_receipt_verified", "payment", payments[i])

		if s.db != nil {
			savedPayment := payments[i]
			db := s.db
			go func() {
				_ = db.SavePaymentToDB(context.Background(), userID, savedPayment)
			}()
		}

		return payments[i], nil
	}

	return Payment{}, errors.New("payment not found")
}
