package platform

import (
	"context"
	"fmt"
	"log"
	"time"

	"FINIX/backend/internal/domain/transaction"
	"FINIX/backend/internal/infra/repo"
)

type ServicePostgres struct {
	repo *repo.Repo
}

func NewServicePostgres(r *repo.Repo) *ServicePostgres {
	return &ServicePostgres{repo: r}
}

func (sp *ServicePostgres) SaveUserToDB(ctx context.Context, u User) error {
	_, err := sp.repo.CreateUser(ctx, u.Name, u.Mobile, u.Email, "", u.UBT)
	if err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

func (sp *ServicePostgres) LoadUserFromDB(ctx context.Context, phone string) (*User, error) {
	row, err := sp.repo.GetUserByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:               row.ID,
		Name:             row.Name,
		Mobile:           row.Phone,
		Email:            row.Email,
		KIN:              row.KIN,
		UBT:              row.UBT,
		EKYCVerified:     row.EKYCVerified,
		BiometricEnabled: row.BiometricEnabled,
		DeviceBound:      row.DeviceBound,
		SIMBound:         row.SIMBound,
		NudgePreference:  row.NudgePreference,
		CreatedAt:        row.CreatedAt,
	}, nil
}

func (sp *ServicePostgres) SaveSessionToDB(ctx context.Context, userID, token string, start, expiry time.Time) error {
	return sp.repo.CreateSession(ctx, userID, token, start, expiry)
}

func (sp *ServicePostgres) LoadSessionsFromDB(ctx context.Context) (map[string]string, map[string]time.Time, map[string]time.Time, error) {
	tokens, starts, expiries, err := sp.repo.ListActiveSessions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return tokens, starts, expiries, nil
}

func (sp *ServicePostgres) SaveTransactionToDB(ctx context.Context, userID string, t Transaction) error {
	coolingOff := time.Time{}
	if t.CoolingOffUntil != nil {
		coolingOff = *t.CoolingOffUntil
	}
	_, err := sp.repo.CreateTransaction(ctx, repo.TransactionRow{
		UserID:           userID,
		AmountPaise:      t.AmountPaise,
		Currency:         t.Currency,
		DebitCredit:      t.DebitCredit,
		Recipient:        t.Recipient,
		MerchantName:     t.MerchantName,
		MerchantCategory: t.MerchantCategory,
		Channel:          t.Channel,
		Description:      t.Description,
		Status:           t.Status,
		AssignedCategory: t.AssignedCategory,
		UserNotes:        t.UserNotes,
		RiskLevel:        string(t.RiskLevel),
		RiskScore:        t.RiskScore,
		XAIReason:        t.XAIReason,
		LinkedAccount:    t.LinkedAccount,
		PaymentIntentID:  t.PaymentIntentID,
		UPIIntentID:      t.UPIIntentID,
		BankReference:    t.BankReference,
		IdempotencyKey:   t.IdempotencyKey,
		CoolingOffUntil:  coolingOff,
		CreatedAt:        t.CreatedAt,
	})
	return err
}

func (sp *ServicePostgres) LoadTransactionsFromDB(ctx context.Context, userID string) ([]Transaction, error) {
	rows, err := sp.repo.ListTransactionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Transaction, 0, len(rows))
	for _, r := range rows {
		var cooling *time.Time
		if !r.CoolingOffUntil.IsZero() {
			cooling = &r.CoolingOffUntil
		}
		out = append(out, Transaction{
			ID:               r.ID,
			UserID:           r.UserID,
			AmountPaise:      r.AmountPaise,
			Currency:         r.Currency,
			DebitCredit:      r.DebitCredit,
			Recipient:        r.Recipient,
			MerchantName:     r.MerchantName,
			MerchantCategory: r.MerchantCategory,
			Channel:          r.Channel,
			Description:      r.Description,
			Status:           r.Status,
			AssignedCategory: r.AssignedCategory,
			UserNotes:        r.UserNotes,
			RiskLevel:        transaction.RiskLevel(r.RiskLevel),
			RiskScore:        r.RiskScore,
			XAIReason:        r.XAIReason,
			LinkedAccount:    r.LinkedAccount,
			PaymentIntentID:  r.PaymentIntentID,
			UPIIntentID:      r.UPIIntentID,
			BankReference:    r.BankReference,
			CreatedAt:        r.CreatedAt,
			IdempotencyKey:   r.IdempotencyKey,
			CoolingOffUntil:  cooling,
		})
	}
	return out, nil
}

func (sp *ServicePostgres) SaveInvestmentToDB(ctx context.Context, inv InvestmentHolding, userID string) error {
	_, err := sp.repo.CreateInvestment(ctx, repo.InvestmentRow{
		UserID:                       userID,
		Name:                         inv.Name,
		InstrumentName:               inv.InstrumentName,
		Category:                     inv.Category,
		QuantityUnits:                inv.QuantityUnits,
		AvgPurchasePricePaise:        inv.AvgPurchasePricePaise,
		CurrentValuePaise:            inv.CurrentValuePaise,
		InvestedPaise:                inv.InvestedPaise,
		LastValuationDate:            inv.LastValuationDate,
		SIPAmountPaise:               inv.SIPAmountPaise,
		SIPFrequency:                 inv.SIPFrequency,
		SIPStartDate:                 inv.SIPStartDate,
		SIPStatus:                    inv.SIPStatus,
		BrokerFundHouse:              inv.BrokerFundHouse,
		DividendsPaise:               inv.DividendsPaise,
		TaxLotDate:                   inv.TaxLotDate,
		CAGR:                         inv.CAGR,
		RecommendationID:             inv.RecommendationID,
		RecommendationExplainability: inv.RecommendationExplainability,
		CreatedAt:                    inv.CreatedAt,
	})
	return err
}

func (sp *ServicePostgres) LoadInvestmentsFromDB(ctx context.Context, userID string) ([]InvestmentHolding, error) {
	rows, err := sp.repo.ListInvestmentsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]InvestmentHolding, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvestmentHolding{
			ID:                           r.ID,
			Name:                         r.Name,
			InstrumentName:               r.InstrumentName,
			Category:                     r.Category,
			QuantityUnits:                r.QuantityUnits,
			AvgPurchasePricePaise:        r.AvgPurchasePricePaise,
			CurrentValuePaise:            r.CurrentValuePaise,
			InvestedPaise:                r.InvestedPaise,
			LastValuationDate:            r.LastValuationDate,
			SIPAmountPaise:               r.SIPAmountPaise,
			SIPFrequency:                 r.SIPFrequency,
			SIPStartDate:                 r.SIPStartDate,
			SIPStatus:                    r.SIPStatus,
			BrokerFundHouse:              r.BrokerFundHouse,
			DividendsPaise:               r.DividendsPaise,
			TaxLotDate:                   r.TaxLotDate,
			CAGR:                         r.CAGR,
			RecommendationID:             r.RecommendationID,
			RecommendationExplainability: r.RecommendationExplainability,
			CreatedAt:                    r.CreatedAt,
		})
	}
	return out, nil
}

func (sp *ServicePostgres) SaveGoalToDB(ctx context.Context, g Goal, userID string) error {
	_, err := sp.repo.CreateGoal(ctx, repo.GoalRow{
		UserID:                   userID,
		Name:                     g.Name,
		Description:              g.Description,
		TargetAmountPaise:        g.TargetAmountPaise,
		Currency:                 g.Currency,
		SavedAmountPaise:         g.SavedAmountPaise,
		MonthlyContributionPaise: g.MonthlyContributionPaise,
		Frequency:                g.Frequency,
		Priority:                 g.Priority,
		StartDate:                g.StartDate,
		TargetDate:               g.TargetDate,
		Status:                   g.Status,
		LinkedAccount:            g.LinkedAccount,
		CreatedAt:                g.CreatedAt,
	})
	return err
}

func (sp *ServicePostgres) LoadGoalsFromDB(ctx context.Context, userID string) ([]Goal, error) {
	rows, err := sp.repo.ListGoalsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Goal, 0, len(rows))
	for _, r := range rows {
		out = append(out, Goal{
			ID:                       r.ID,
			Name:                     r.Name,
			Description:              r.Description,
			TargetAmountPaise:        r.TargetAmountPaise,
			Currency:                 r.Currency,
			SavedAmountPaise:         r.SavedAmountPaise,
			MonthlyContributionPaise: r.MonthlyContributionPaise,
			Frequency:                r.Frequency,
			Priority:                 r.Priority,
			StartDate:                r.StartDate,
			TargetDate:               r.TargetDate,
			Status:                   r.Status,
			LinkedAccount:            r.LinkedAccount,
			CreatedAt:                r.CreatedAt,
		})
	}
	return out, nil
}

func (sp *ServicePostgres) SaveAccountToDB(ctx context.Context, a Account, userID string) error {
	_, err := sp.repo.CreateAccount(ctx, repo.AccountRow{
		UserID:              userID,
		AccountHolderName:   a.AccountHolderName,
		BankName:            a.BankName,
		Branch:              a.Branch,
		IFSCCode:            a.IFSCCode,
		MaskedAccountNumber: a.AccountNumberMaskedToken,
		UPIID:               a.UPIID,
		AccountType:         a.AccountType,
		VerificationStatus:  a.VerificationStatus,
		Nickname:            a.Nickname,
		PrimaryAccountFlag:  a.PrimaryAccountFlag,
		AccountToken:        a.AccountToken,
		TokenProvider:       a.TokenProvider,
		BalancePaise:        a.BalancePaise,
		LinkedAt:            a.LinkedAt,
	})
	return err
}

func (sp *ServicePostgres) LoadAccountsFromDB(ctx context.Context, userID string) ([]Account, error) {
	var rows []repo.AccountRow
	var err error

	if userID == "*" {
		rows, err = sp.repo.ListAllAccounts(ctx)
	} else {
		rows, err = sp.repo.ListAccountsByUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, Account{
			ID:                       r.ID,
			AccountHolderName:        r.AccountHolderName,
			BankName:                 r.BankName,
			Branch:                   r.Branch,
			IFSCCode:                 r.IFSCCode,
			AccountNumberMaskedToken: r.MaskedAccountNumber,
			UPIID:                    r.UPIID,
			AccountType:              r.AccountType,
			VerificationStatus:       r.VerificationStatus,
			Nickname:                 r.Nickname,
			PrimaryAccountFlag:       r.PrimaryAccountFlag,
			AccountToken:             r.AccountToken,
			TokenProvider:            r.TokenProvider,
			BalancePaise:             r.BalancePaise,
			LinkedAt:                 r.LinkedAt,
		})
	}
	return out, nil
}

func (sp *ServicePostgres) SaveFreezeStateToDB(ctx context.Context, userID string, frozen bool) error {
	return sp.repo.SetFreezeState(ctx, userID, frozen)
}

func (sp *ServicePostgres) LoadFreezeStateFromDB(ctx context.Context, userID string) (bool, error) {
	return sp.repo.GetFreezeState(ctx, userID)
}

func (sp *ServicePostgres) SaveAuditEventToDB(ctx context.Context, userID string, e AuditEvent) error {
	_, err := sp.repo.CreateAuditEvent(ctx, repo.AuditEventRow{
		UserID:      userID,
		EventType:   e.EventType,
		TriggeredBy: e.TriggeredBy,
		Outcome:     e.Outcome,
		Details:     e.Details,
		XAIReason:   e.XAIReason,
		CreatedAt:   e.Timestamp,
	})
	return err
}

func (sp *ServicePostgres) SavePaymentToDB(ctx context.Context, userID string, p Payment) error {
	var completedAt *time.Time
	if p.CompletedAt != nil {
		completedAt = p.CompletedAt
	}
	var receiptVerifiedAt *time.Time
	var receiptHash string
	if p.ReceiptVerification != nil {
		receiptVerifiedAt = p.ReceiptVerification.VerifiedAt
		receiptHash = p.ReceiptVerification.ReceiptHash
	}
	_, err := sp.repo.CreatePayment(ctx, repo.PaymentRow{
		UserID:               userID,
		BeneficiaryID:        p.BeneficiaryID,
		BeneficiaryName:      p.BeneficiaryName,
		BeneficiaryAccount:   p.BeneficiaryAccount,
		AmountPaise:          p.AmountPaise,
		Currency:             p.Currency,
		Method:               p.Method,
		Status:               p.Status,
		RiskLevel:            p.RiskLevel,
		RiskScore:            p.RiskScore,
		StartedAt:            p.StartedAt,
		CompletedAt:          completedAt,
		OTPVerified:          p.OTPVerified,
		UserConsent:          p.UserConsent,
		ReceiptVerified:      p.ReceiptVerification != nil && p.ReceiptVerification.Verified,
		ReceiptVerifiedAt:    receiptVerifiedAt,
		ReceiptHash:          receiptHash,
		PaymentIntentID:      p.PaymentIntentID,
		UPIIntentID:          p.UPIIntentID,
		QRCodePayload:        p.QRCodePayload,
		SIPID:                p.SIPID,
		StepUpChallengeID:    p.StepUpChallengeID,
		DeviceChallengeKeyID: p.DeviceChallengeKeyID,
		CreatedAt:            p.CreatedAt,
	})
	return err
}

func (sp *ServicePostgres) LoadPaymentsFromDB(ctx context.Context, userID string) ([]Payment, error) {
	rows, err := sp.repo.ListPaymentsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Payment, 0, len(rows))
	for _, r := range rows {
		var receiptVerification *ReceiptVerification
		if r.ReceiptVerified {
			receiptVerification = &ReceiptVerification{
				Verified:    true,
				VerifiedAt:  r.ReceiptVerifiedAt,
				ReceiptHash: r.ReceiptHash,
			}
		}
		out = append(out, Payment{
			ID:                   r.ID,
			UserID:               r.UserID,
			BeneficiaryID:        r.BeneficiaryID,
			BeneficiaryName:      r.BeneficiaryName,
			BeneficiaryAccount:   r.BeneficiaryAccount,
			AmountPaise:          r.AmountPaise,
			Currency:             r.Currency,
			Method:               r.Method,
			Status:               r.Status,
			RiskLevel:            r.RiskLevel,
			RiskScore:            r.RiskScore,
			StartedAt:            r.StartedAt,
			CompletedAt:          r.CompletedAt,
			BankReference:        r.BankReference,
			OTPVerified:          r.OTPVerified,
			UserConsent:          r.UserConsent,
			RefundInfo:           nil,
			PaymentIntentID:      r.PaymentIntentID,
			UPIIntentID:          r.UPIIntentID,
			QRCodePayload:        r.QRCodePayload,
			ReceiptVerification:  receiptVerification,
			SIPID:                r.SIPID,
			PaymentTimelineID:    "",
			StepUpChallengeID:    r.StepUpChallengeID,
			DeviceChallengeKeyID: r.DeviceChallengeKeyID,
			CreatedAt:            r.CreatedAt,
		})
	}
	return out, nil
}

func (sp *ServicePostgres) SaveBlockchainEventToDB(ctx context.Context, userID string, ev BlockchainEvent) error {
	return sp.repo.WriteBlockchainEvent(ctx, userID, ev.Action, ev.Resource, ev.PayloadHash, ev.PrevHash, ev.Hash)
}

func (sp *ServicePostgres) SaveConsentToDB(ctx context.Context, userID, consentType string, granted bool) error {
	return sp.repo.UpsertConsentGrant(ctx, userID, consentType, granted)
}

func (sp *ServicePostgres) SaveBeneficiaryLinkToDB(ctx context.Context, userID, recipientHash string) error {
	return sp.repo.CreateBeneficiaryLink(ctx, userID, recipientHash)
}

type BlockchainEvent struct {
	Action      string
	Resource    string
	PayloadHash string
	PrevHash    string
	Hash        string
}

func (sp *ServicePostgres) SyncToDatabase(ctx context.Context, svc *Service) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	for _, u := range svc.users {
		if err := sp.SaveUserToDB(ctx, *u); err != nil {
			log.Printf("[DB SYNC] save user %s: %v", u.ID, err)
		}
	}

	for token, userID := range svc.sessions {
		start := svc.sessionStart[token]
		expiry := svc.sessionExpiry[token]
		if err := sp.SaveSessionToDB(ctx, userID, token, start, expiry); err != nil {
			log.Printf("[DB SYNC] save session %s: %v", token, err)
		}
	}

	for userID, txns := range svc.transactions {
		for _, txn := range txns {
			if err := sp.SaveTransactionToDB(ctx, userID, txn); err != nil {
				log.Printf("[DB SYNC] save txn %s: %v", txn.ID, err)
			}
		}
	}

	for userID, goals := range svc.goals {
		for _, g := range goals {
			if err := sp.SaveGoalToDB(ctx, g, userID); err != nil {
				log.Printf("[DB SYNC] save goal %s: %v", g.ID, err)
			}
		}
	}

	for userID, accts := range svc.accounts {
		for _, a := range accts {
			if err := sp.SaveAccountToDB(ctx, a, userID); err != nil {
				log.Printf("[DB SYNC] save account %s: %v", a.ID, err)
			}
		}
	}

	for userID, invs := range svc.investments {
		for _, inv := range invs {
			if err := sp.SaveInvestmentToDB(ctx, inv, userID); err != nil {
				log.Printf("[DB SYNC] save investment %s: %v", inv.ID, err)
			}
		}
	}

	for userID, frozen := range svc.freezeState {
		if err := sp.SaveFreezeStateToDB(ctx, userID, frozen); err != nil {
			log.Printf("[DB SYNC] save freeze %s: %v", userID, err)
		}
	}

	for userID, consents := range svc.consents {
		for ct, granted := range consents {
			if err := sp.SaveConsentToDB(ctx, userID, ct, granted); err != nil {
				log.Printf("[DB SYNC] save consent %s/%s: %v", userID, ct, err)
			}
		}
	}

	for userID, contacts := range svc.emergencyContacts {
		for _, c := range contacts {
			if err := sp.SaveEmergencyContactToDB(ctx, c, userID); err != nil {
				log.Printf("[DB SYNC] save emergency contact %s: %v", c.ID, err)
			}
		}
	}

	for userID, assets := range svc.netWorthAssets {
		for _, a := range assets {
			if err := sp.SaveNetWorthAssetToDB(ctx, a, userID); err != nil {
				log.Printf("[DB SYNC] save net worth asset %s: %v", a.ID, err)
			}
		}
	}

	return nil
}

func (sp *ServicePostgres) SaveEmergencyContactToDB(ctx context.Context, c EmergencyContact, userID string) error {
	_, err := sp.repo.CreateEmergencyContact(ctx, repo.EmergencyContactRow{
		ID: c.ID, UserID: userID, Name: c.Name,
		Relationship: c.Relationship, Phone: c.Phone, Email: c.Email,
	}, userID)
	if err != nil {
		return fmt.Errorf("save emergency contact: %w", err)
	}
	return nil
}

func (sp *ServicePostgres) SaveNetWorthAssetToDB(ctx context.Context, a NetWorthAsset, userID string) error {
	_, err := sp.repo.CreateNetWorthAsset(ctx, repo.NetWorthAssetRow{
		ID: a.ID, UserID: userID, Type: a.Type,
		Description: a.Description, ValuePaise: a.ValuePaise,
		ValuationDate: a.ValuationDate, ValuationSource: a.ValuationSource,
	}, userID)
	if err != nil {
		return fmt.Errorf("save net worth asset: %w", err)
	}
	return nil
}
