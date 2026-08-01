package platform

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// unfreezeOTPRequired reports whether lifting an emergency freeze needs a
// verified OTP in addition to the biometric factor. It follows the same
// FINIX_REQUIRE_BIOMETRIC_CHALLENGE toggle used by the API's biometric gate, so
// a single switch selects demo vs production step-up posture.
func unfreezeOTPRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_REQUIRE_BIOMETRIC_CHALLENGE"))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// Frontend compatibility layer.
//
// The Flutter client (PSBFRONT lib/services/api_service.dart and the screens
// that consume it) reads a handful of fields under names that differ from this
// package's canonical JSON tags — e.g. it reads `netWorth` where we emit
// `netWorthPaise`, and `balance` where we emit `balancePaise`.
//
// Rather than renaming the canonical fields (which would break existing API
// consumers, tests and the OpenAPI surface), each type below marshals its
// normal payload PLUS additive alias keys. Both spellings are present in the
// response, so old and new clients both work.
//
// Implementation note: each MarshalJSON declares `type alias T` to shed the
// method set (avoiding infinite recursion) and embeds it so its fields are
// promoted inline alongside the alias keys.

// --- Net worth ---------------------------------------------------------

// MarshalJSON adds netWorth / totalAssets / totalLiabilities aliases.
// The dashboard reads data['netWorth'] (paise).
func (s NetWorthSnapshot) MarshalJSON() ([]byte, error) {
	type alias NetWorthSnapshot
	return json.Marshal(struct {
		alias
		NetWorth         int64 `json:"netWorth"`
		TotalAssets      int64 `json:"totalAssets"`
		TotalLiabilities int64 `json:"totalLiabilities"`
	}{
		alias:            alias(s),
		NetWorth:         s.NetWorthPaise,
		TotalAssets:      s.TotalAssetsPaise,
		TotalLiabilities: s.TotalLiabilitiesPaise,
	})
}

// MarshalJSON adds a `name` alias (the client lists assets by name).
func (a NetWorthAsset) MarshalJSON() ([]byte, error) {
	type alias NetWorthAsset
	return json.Marshal(struct {
		alias
		Name string `json:"name"`
	}{alias: alias(a), Name: a.Description})
}

// MarshalJSON adds `name` and `valuePaise` aliases so liabilities render with
// the same accessors as assets.
func (l NetWorthLiability) MarshalJSON() ([]byte, error) {
	type alias NetWorthLiability
	return json.Marshal(struct {
		alias
		Name       string `json:"name"`
		ValuePaise int64  `json:"valuePaise"`
	}{alias: alias(l), Name: l.Description, ValuePaise: l.OutstandingPaise})
}

// --- Accounts ----------------------------------------------------------

// MarshalJSON adds accountId / balance / accountNumber aliases.
// linked_accounts.dart reads apiAcc['balance'].
func (a Account) MarshalJSON() ([]byte, error) {
	type alias Account
	return json.Marshal(struct {
		alias
		AccountID     string `json:"accountId"`
		Balance       int64  `json:"balance"`
		AccountNumber string `json:"accountNumber"`
	}{
		alias:         alias(a),
		AccountID:     a.ID,
		Balance:       a.BalancePaise,
		AccountNumber: a.AccountNumberMaskedToken,
	})
}

// --- Beneficiaries -----------------------------------------------------

// MarshalJSON adds beneficiaryId / name / upiId aliases.
func (b Beneficiary) MarshalJSON() ([]byte, error) {
	type alias Beneficiary
	return json.Marshal(struct {
		alias
		BeneficiaryID string `json:"beneficiaryId"`
		Name          string `json:"name"`
		UPIID         string `json:"upiId"`
	}{
		alias:         alias(b),
		BeneficiaryID: b.ID,
		Name:          b.BeneficiaryName,
		UPIID:         b.UPIIDOrBankDetails,
	})
}

// --- Transactions ------------------------------------------------------

// MarshalJSON adds type / category / timestamp aliases. The dashboard reads
// txn['type'] == 'debit', txn['category'] and txn['merchantName'].
func (t Transaction) MarshalJSON() ([]byte, error) {
	type alias Transaction
	category := t.AssignedCategory
	if category == "" {
		category = t.MerchantCategory
	}
	if category == "" {
		category = "Payment"
	}
	return json.Marshal(struct {
		alias
		Type      string    `json:"type"`
		Category  string    `json:"category"`
		Timestamp time.Time `json:"timestamp"`
	}{
		alias:     alias(t),
		Type:      t.DebitCredit,
		Category:  category,
		Timestamp: t.CreatedAt,
	})
}

// --- Goals -------------------------------------------------------------

// MarshalJSON adds a goalId alias (goals.dart reads apiGoal['goalId']).
func (g Goal) MarshalJSON() ([]byte, error) {
	type alias Goal
	return json.Marshal(struct {
		alias
		GoalID string `json:"goalId"`
	}{alias: alias(g), GoalID: g.ID})
}

// --- Investments & loans ----------------------------------------------

// MarshalJSON adds units / SIPAmountPaise aliases.
func (h InvestmentHolding) MarshalJSON() ([]byte, error) {
	type alias InvestmentHolding
	return json.Marshal(struct {
		alias
		Units          float64 `json:"units"`
		SIPAmountPaise int64   `json:"SIPAmountPaise"`
	}{alias: alias(h), Units: h.QuantityUnits, SIPAmountPaise: h.SIPAmountPaise})
}

// MarshalJSON adds a tenureRemainingMonths alias.
func (l LoanRecord) MarshalJSON() ([]byte, error) {
	type alias LoanRecord
	return json.Marshal(struct {
		alias
		TenureRemainingMonths int `json:"tenureRemainingMonths"`
	}{alias: alias(l), TenureRemainingMonths: l.RemainingMonths})
}

// --- Insurance ---------------------------------------------------------

// InsurancePortfolio is the object shape the client expects from
// GET /v1/portfolio/insurance: aggregate cover plus the policy list. The
// endpoint previously returned a bare array, which the client could not decode
// as a map (it reads data['policies'] / data['totalLifeCoverPaise']).
type InsurancePortfolio struct {
	TotalLifeCoverPaise   int64             `json:"totalLifeCoverPaise"`
	TotalHealthCoverPaise int64             `json:"totalHealthCoverPaise"`
	TotalCoverPaise       int64             `json:"totalCoverPaise"`
	Policies              []InsurancePolicy `json:"policies"`
}

// BuildInsurancePortfolio aggregates policies into the client-facing object.
// Life-type policies (term/life/endowment) roll into the life cover total; the
// rest (health/critical illness) roll into health cover.
func BuildInsurancePortfolio(policies []InsurancePolicy) InsurancePortfolio {
	out := InsurancePortfolio{Policies: policies}
	if out.Policies == nil {
		out.Policies = []InsurancePolicy{}
	}
	for _, p := range policies {
		if isLifeCover(p.PolicyType) {
			out.TotalLifeCoverPaise += p.SumAssuredPaise
		} else {
			out.TotalHealthCoverPaise += p.SumAssuredPaise
		}
	}
	out.TotalCoverPaise = out.TotalLifeCoverPaise + out.TotalHealthCoverPaise
	return out
}

func isLifeCover(policyType string) bool {
	switch policyType {
	case "term_life", "life", "endowment", "ulip", "term":
		return true
	default:
		return false
	}
}

// --- Auth profile ------------------------------------------------------

// MarshalJSON adds a userId alias for InternalUserID (the client reads
// profile['userId']).
func (a AuthProfile) MarshalJSON() ([]byte, error) {
	type alias AuthProfile
	return json.Marshal(struct {
		alias
		UserID string `json:"userId"`
	}{alias: alias(a), UserID: a.InternalUserID})
}

// --- Security, chat and tax -------------------------------------------

// MarshalJSON adds is_frozen / account_frozen aliases. security.dart reads
// health['is_frozen'] to drive the freeze toggle.
func (h SecurityHealth) MarshalJSON() ([]byte, error) {
	type alias SecurityHealth
	return json.Marshal(struct {
		alias
		IsFrozen      bool `json:"is_frozen"`
		AccountFrozen bool `json:"account_frozen"`
	}{alias: alias(h), IsFrozen: h.AccountFrozen, AccountFrozen: h.AccountFrozen})
}

// MarshalJSON adds a `content` alias for the reply text (chat.dart reads
// response['content']) plus confidenceScore.
func (c ChatResponse) MarshalJSON() ([]byte, error) {
	type alias ChatResponse
	return json.Marshal(struct {
		alias
		Content         string  `json:"content"`
		ConfidenceScore float64 `json:"confidenceScore"`
	}{alias: alias(c), Content: c.Reply, ConfidenceScore: c.ModelConfidence})
}

// MarshalJSON adds flattened taxPayable / regime / deductions aliases.
// tax_centre.dart reads taxData['taxPayable'], ['regime'] and ['deductions']
// (the latter as a single paise total, not the per-section breakdown).
func (t TaxDashboard) MarshalJSON() ([]byte, error) {
	type alias TaxDashboard
	var deductionsTotal int64
	for _, d := range t.Deductions {
		deductionsTotal += d.UsedPaise
	}
	return json.Marshal(struct {
		alias
		TaxableIncome int64  `json:"taxableIncome"`
		TaxPayable    int64  `json:"taxPayable"`
		DeductionsTot int64  `json:"deductions"`
		Regime        string `json:"regime"`
	}{
		alias:         alias(t),
		TaxableIncome: t.TaxableIncomePaise,
		TaxPayable:    t.EstimatedTaxLiabilityPaise,
		DeductionsTot: deductionsTotal,
		Regime:        t.TaxRegime,
	})
}

// RegimeBreakdown is the per-regime object the client renders.
type RegimeBreakdown struct {
	TaxableIncome int64 `json:"taxableIncome"`
	TaxPayable    int64 `json:"taxPayable"`
	Deductions    int64 `json:"deductions"`
}

// MarshalJSON adds nested oldRegime / newRegime objects alongside the flat
// *TaxPaise fields.
func (c TaxRegimeComparison) MarshalJSON() ([]byte, error) {
	type alias TaxRegimeComparison
	return json.Marshal(struct {
		alias
		OldRegime RegimeBreakdown `json:"oldRegime"`
		NewRegime RegimeBreakdown `json:"newRegime"`
	}{
		alias:     alias(c),
		OldRegime: RegimeBreakdown{TaxPayable: c.OldRegimeTaxPaise},
		NewRegime: RegimeBreakdown{TaxPayable: c.NewRegimeTaxPaise},
	})
}

// MarshalJSON adds provider / type / coverAmountPaise aliases per policy.
func (p InsurancePolicy) MarshalJSON() ([]byte, error) {
	type alias InsurancePolicy
	return json.Marshal(struct {
		alias
		ID               string `json:"id"`
		Provider         string `json:"provider"`
		Type             string `json:"type"`
		CoverAmountPaise int64  `json:"coverAmountPaise"`
		DueDate          string `json:"dueDate"`
	}{
		alias:            alias(p),
		ID:               p.PolicyID,
		Provider:         p.Insurer,
		Type:             p.PolicyType,
		CoverAmountPaise: p.SumAssuredPaise,
		DueDate:          p.NextDueDate,
	})
}
