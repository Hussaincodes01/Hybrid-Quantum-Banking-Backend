package platform

import (
	"encoding/json"
	"testing"
	"time"
)

// These tests lock the field names the Flutter client (PSBFRONT) reads. If a
// canonical field is renamed without updating the alias, the client silently
// falls back to mock data — so assert the alias keys explicitly.

func keys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func requireKeys(t *testing.T, v any, want ...string) {
	t.Helper()
	m := keys(t, v)
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("response is missing client-required key %q", k)
		}
	}
}

func TestNetWorthSnapshotClientAliases(t *testing.T) {
	s := NetWorthSnapshot{
		TotalAssetsPaise:      500,
		TotalLiabilitiesPaise: 200,
		NetWorthPaise:         300,
		Assets:                []NetWorthAsset{{Description: "SBI savings", ValuePaise: 500}},
		Liabilities:           []NetWorthLiability{{Description: "Home loan", OutstandingPaise: 200}},
	}
	requireKeys(t, s, "netWorth", "totalAssets", "totalLiabilities",
		"netWorthPaise", "totalAssetsPaise", "totalLiabilitiesPaise")

	m := keys(t, s)
	var netWorth int64
	_ = json.Unmarshal(m["netWorth"], &netWorth)
	if netWorth != 300 {
		t.Errorf("netWorth = %d, want 300", netWorth)
	}
	// assets[].name and liabilities[].valuePaise are read by the client.
	requireKeys(t, s.Assets[0], "name", "valuePaise")
	requireKeys(t, s.Liabilities[0], "name", "valuePaise")
}

func TestAccountClientAliases(t *testing.T) {
	a := Account{ID: "acct_1", BalancePaise: 1234, AccountNumberMaskedToken: "****9088", BankName: "HDFC"}
	requireKeys(t, a, "accountId", "balance", "accountNumber", "bankName", "accountType")
	m := keys(t, a)
	var bal int64
	_ = json.Unmarshal(m["balance"], &bal)
	if bal != 1234 {
		t.Errorf("balance = %d, want 1234", bal)
	}
}

func TestTransactionClientAliases(t *testing.T) {
	now := time.Now().UTC()
	tx := Transaction{ID: "txn_1", DebitCredit: "debit", AmountPaise: 500, MerchantName: "Zomato", CreatedAt: now}
	requireKeys(t, tx, "type", "category", "timestamp", "amountPaise", "merchantName")

	m := keys(t, tx)
	var typ, cat string
	_ = json.Unmarshal(m["type"], &typ)
	_ = json.Unmarshal(m["category"], &cat)
	if typ != "debit" {
		t.Errorf("type = %q, want debit", typ)
	}
	if cat == "" {
		t.Error("category must never be empty (client renders it directly)")
	}
}

func TestGoalBeneficiaryLoanInvestmentAliases(t *testing.T) {
	requireKeys(t, Goal{ID: "goal_1"}, "goalId", "name", "targetAmountPaise", "savedAmountPaise")
	requireKeys(t, Beneficiary{ID: "ben_1", BeneficiaryName: "A", UPIIDOrBankDetails: "a@upi"},
		"beneficiaryId", "name", "upiId", "trustScore")
	requireKeys(t, LoanRecord{LoanID: "loan_1", RemainingMonths: 36},
		"tenureRemainingMonths", "outstandingPaise", "emiPaise", "lender")
	requireKeys(t, InvestmentHolding{ID: "inv_1", QuantityUnits: 10, SIPAmountPaise: 500},
		"units", "SIPAmountPaise", "currentValuePaise")
}

func TestInsurancePortfolioShape(t *testing.T) {
	// The client decodes this endpoint as an OBJECT and reads the cover totals,
	// so a bare array would break it.
	p := BuildInsurancePortfolio([]InsurancePolicy{
		{PolicyID: "p1", PolicyType: "term_life", Insurer: "LIC", SumAssuredPaise: 1000},
		{PolicyID: "p2", PolicyType: "health", Insurer: "Star", SumAssuredPaise: 400},
	})
	requireKeys(t, p, "totalLifeCoverPaise", "totalHealthCoverPaise", "policies")
	if p.TotalLifeCoverPaise != 1000 {
		t.Errorf("life cover = %d, want 1000", p.TotalLifeCoverPaise)
	}
	if p.TotalHealthCoverPaise != 400 {
		t.Errorf("health cover = %d, want 400", p.TotalHealthCoverPaise)
	}
	requireKeys(t, p.Policies[0], "provider", "type", "coverAmountPaise", "dueDate")
}

func TestSecurityChatTaxProfileAliases(t *testing.T) {
	requireKeys(t, SecurityHealth{AccountFrozen: true}, "is_frozen", "account_frozen")
	m := keys(t, SecurityHealth{AccountFrozen: true})
	var frozen bool
	_ = json.Unmarshal(m["is_frozen"], &frozen)
	if !frozen {
		t.Error("is_frozen must reflect AccountFrozen")
	}

	requireKeys(t, ChatResponse{Reply: "hello", ModelConfidence: 0.9}, "content", "suggestions", "confidenceScore")
	cm := keys(t, ChatResponse{Reply: "hello"})
	var content string
	_ = json.Unmarshal(cm["content"], &content)
	if content != "hello" {
		t.Errorf("content = %q, want the reply text", content)
	}

	requireKeys(t, TaxDashboard{TaxableIncomePaise: 10, EstimatedTaxLiabilityPaise: 3,
		TaxRegime: "new", Deductions: []Deduction{{UsedPaise: 7}}},
		"taxPayable", "regime", "deductions", "taxableIncome")

	requireKeys(t, TaxRegimeComparison{OldRegimeTaxPaise: 5, NewRegimeTaxPaise: 3, Recommended: "new"},
		"oldRegime", "newRegime", "recommended")

	requireKeys(t, AuthProfile{InternalUserID: "usr_1"}, "userId", "kycVerified", "biometricEnabled")
}

func TestUnfreezeAcceptsVerificationMethod(t *testing.T) {
	// The client posts {"verificationMethod":"biometric"} rather than biometricOk.
	if !(UnfreezeRequest{VerificationMethod: "biometric"}).biometricSatisfied() {
		t.Error(`verificationMethod:"biometric" must satisfy the biometric factor`)
	}
	if !(UnfreezeRequest{BiometricOK: true}).biometricSatisfied() {
		t.Error("biometricOk:true must satisfy the biometric factor")
	}
	if (UnfreezeRequest{}).biometricSatisfied() {
		t.Error("an empty request must NOT satisfy the biometric factor")
	}
	if (UnfreezeRequest{VerificationMethod: "sms"}).biometricSatisfied() {
		t.Error("a non-biometric method must NOT satisfy the biometric factor")
	}
}
