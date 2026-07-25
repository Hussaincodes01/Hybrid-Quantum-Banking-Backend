package platform

import (
	"context"
	"log"
	"time"

	"FINIX/backend/internal/domain/transaction"
)

func (s *Service) HydrateFromDB(ctx context.Context) error {
	if s.db == nil {
		return nil
	}

	log.Println("[HYDRATE] loading data from PostgreSQL...")

	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.db.repo.ListAllUsers(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list users: %v", err)
	} else {
		for _, u := range users {
			s.users[u.ID] = &User{
				ID:               u.ID,
				Name:             u.Name,
				Mobile:           u.Phone,
				Email:            u.Email,
				KIN:              u.KIN,
				UBT:              u.UBT,
				EKYCVerified:     u.EKYCVerified,
				BiometricEnabled: u.BiometricEnabled,
				DeviceBound:      u.DeviceBound,
				SIMBound:         u.SIMBound,
				NudgePreference:  u.NudgePreference,
				CreatedAt:        u.CreatedAt,
			}
			s.mobileIndex[u.Phone] = u.ID
			s.beneficiaries[u.ID] = make(map[string]struct{})
			s.beneficiaryRecords[u.ID] = []Beneficiary{}
		}
		log.Printf("[HYDRATE] loaded %d users", len(users))
	}

	sessions, starts, expiries, err := s.db.repo.ListActiveSessions(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list sessions: %v", err)
	} else {
		for token, uid := range sessions {
			s.sessions[token] = uid
			if start, ok := starts[token]; ok {
				s.sessionStart[token] = start
			}
			if expiry, ok := expiries[token]; ok {
				s.sessionExpiry[token] = expiry
			}
		}
		log.Printf("[HYDRATE] loaded %d active sessions", len(sessions))
	}

	freezeStates, err := s.db.repo.ListFreezeStates(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list freeze states: %v", err)
	} else {
		for uid, frozen := range freezeStates {
			s.freezeState[uid] = frozen
		}
		log.Printf("[HYDRATE] loaded %d freeze states", len(freezeStates))
	}

	consents, err := s.db.repo.ListAllConsentGrants(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list consents: %v", err)
	} else {
		for uid, grants := range consents {
			s.consents[uid] = grants
		}
		log.Printf("[HYDRATE] loaded consents for %d users", len(consents))
	}

	notifications, err := s.db.repo.ListAllNotificationSettings(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list notification settings: %v", err)
	} else {
		for uid, notifs := range notifications {
			settings := make([]NotificationSetting, 0, len(notifs))
			for _, n := range notifs {
				settings = append(settings, NotificationSetting{
					Category: n.Category,
					Enabled:  n.Enabled,
				})
			}
			s.notifications[uid] = settings
		}
		log.Printf("[HYDRATE] loaded notification settings for %d users", len(notifications))
	}

	acctRows, err := s.db.repo.ListAllAccounts(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list accounts: %v", err)
	} else {
		acctByUser := make(map[string][]Account)
		for _, r := range acctRows {
			acctByUser[r.UserID] = append(acctByUser[r.UserID], Account{
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
		for uid, accts := range acctByUser {
			s.accounts[uid] = accts
		}
		log.Printf("[HYDRATE] loaded %d accounts across %d users", len(acctRows), len(acctByUser))
	}

	txnRows, err := s.db.repo.ListAllTransactions(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list transactions: %v", err)
	} else {
		allTxns := make(map[string][]Transaction)
		for _, t := range txnRows {
			var cooling *time.Time
			if !t.CoolingOffUntil.IsZero() {
				coolingCopy := t.CoolingOffUntil
				cooling = &coolingCopy
			}
			allTxns[t.UserID] = append(allTxns[t.UserID], Transaction{
				ID:               t.ID,
				UserID:           t.UserID,
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
				RiskLevel:        transaction.RiskLevel(t.RiskLevel),
				RiskScore:        t.RiskScore,
				XAIReason:        t.XAIReason,
				LinkedAccount:    t.LinkedAccount,
				PaymentIntentID:  t.PaymentIntentID,
				UPIIntentID:      t.UPIIntentID,
				BankReference:    t.BankReference,
				CreatedAt:        t.CreatedAt,
				IdempotencyKey:   t.IdempotencyKey,
				CoolingOffUntil:  cooling,
			})
		}
		for userID, txns := range allTxns {
			s.transactions[userID] = txns
		}
		log.Printf("[HYDRATE] loaded %d transactions", len(txnRows))
	}

	goalRows, err := s.db.repo.ListAllGoals(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list goals: %v", err)
	} else {
		allGoals := make(map[string][]Goal)
		for _, g := range goalRows {
			allGoals[g.UserID] = append(allGoals[g.UserID], Goal{
				ID:                       g.ID,
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
		}
		for userID, goals := range allGoals {
			s.goals[userID] = goals
		}
		log.Printf("[HYDRATE] loaded %d goals", len(goalRows))
	}

	paymentRows, err := s.db.repo.ListAllPayments(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list payments: %v", err)
	} else {
		allPayments := make(map[string][]Payment)
		for _, p := range paymentRows {
			var receiptVerification *ReceiptVerification
			if p.ReceiptVerified {
				receiptVerification = &ReceiptVerification{
					Verified:    true,
					VerifiedAt:  p.ReceiptVerifiedAt,
					ReceiptHash: p.ReceiptHash,
				}
			}
			allPayments[p.UserID] = append(allPayments[p.UserID], Payment{
				ID:                   p.ID,
				UserID:               p.UserID,
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
				CompletedAt:          p.CompletedAt,
				BankReference:        p.BankReference,
				OTPVerified:          p.OTPVerified,
				UserConsent:          p.UserConsent,
				PaymentIntentID:      p.PaymentIntentID,
				UPIIntentID:          p.UPIIntentID,
				QRCodePayload:        p.QRCodePayload,
				ReceiptVerification:  receiptVerification,
				SIPID:                p.SIPID,
				StepUpChallengeID:    p.StepUpChallengeID,
				DeviceChallengeKeyID: p.DeviceChallengeKeyID,
				CreatedAt:            p.CreatedAt,
			})
		}
		for userID, payments := range allPayments {
			s.payments[userID] = payments
		}
		log.Printf("[HYDRATE] loaded %d payments across %d users", len(paymentRows), len(allPayments))
	}

	invRows, err := s.db.repo.ListAllInvestments(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list investments: %v", err)
	} else {
		allInvestments := make(map[string][]InvestmentHolding)
		for _, inv := range invRows {
			allInvestments[inv.UserID] = append(allInvestments[inv.UserID], InvestmentHolding{
				ID:                           inv.ID,
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
		}
		for userID, investments := range allInvestments {
			if len(investments) > 0 {
				s.investments[userID] = investments
			}
		}
		log.Printf("[HYDRATE] loaded %d investments", len(invRows))
	}

	// Phase 7 hydrations

	insuranceRows, err := s.db.repo.ListAllInsurance(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list insurance: %v", err)
	} else {
		for userID, rows := range insuranceRows {
			policies := make([]InsurancePolicy, len(rows))
			for i, r := range rows {
				policies[i] = InsurancePolicy{
					PolicyID: r.PolicyID, PolicyType: r.PolicyType,
					Insurer: r.Insurer, SumAssuredPaise: r.SumAssuredPaise,
					PremiumPaise: r.PremiumPaise, NextDueDate: r.NextDueDate,
				}
			}
			s.insurance[userID] = policies
		}
		log.Printf("[HYDRATE] loaded insurance for %d users", len(insuranceRows))
	}

	loanRows, err := s.db.repo.ListAllLoans(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list loans: %v", err)
	} else {
		for userID, rows := range loanRows {
			loans := make([]LoanRecord, len(rows))
			for i, r := range rows {
				loans[i] = LoanRecord{
					LoanID: r.LoanID, Lender: r.Lender, LoanType: r.LoanType,
					OutstandingPaise: r.OutstandingPaise, EMIPaise: r.EMIPaise,
					InterestRate: r.InterestRate, RemainingMonths: r.RemainingMonths,
				}
			}
			s.loans[userID] = loans
		}
		log.Printf("[HYDRATE] loaded loans for %d users", len(loanRows))
	}

	ecRows, err := s.db.repo.ListAllEmergencyContacts(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list emergency contacts: %v", err)
	} else {
		for userID, rows := range ecRows {
			contacts := make([]EmergencyContact, len(rows))
			for i, r := range rows {
				contacts[i] = EmergencyContact{ID: r.ID, Name: r.Name, Relationship: r.Relationship, Phone: r.Phone, Email: r.Email}
			}
			s.emergencyContacts[userID] = contacts
		}
		log.Printf("[HYDRATE] loaded emergency contacts for %d users", len(ecRows))
	}

	nwRows, err := s.db.repo.ListAllNetWorthAssets(ctx)
	if err != nil {
		log.Printf("[HYDRATE] list net worth assets: %v", err)
	} else {
		for userID, rows := range nwRows {
			assets := make([]NetWorthAsset, len(rows))
			for i, r := range rows {
				assets[i] = NetWorthAsset{ID: r.ID, Type: r.Type, Description: r.Description, ValuePaise: r.ValuePaise, ValuationDate: r.ValuationDate, ValuationSource: r.ValuationSource}
			}
			s.netWorthAssets[userID] = assets
		}
		log.Printf("[HYDRATE] loaded net worth assets for %d users", len(nwRows))
	}

	log.Println("[HYDRATE] hydration complete")
	return nil
}
