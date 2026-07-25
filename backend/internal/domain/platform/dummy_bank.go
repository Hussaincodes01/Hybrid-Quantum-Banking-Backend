package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DummyBankAccount struct {
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	BankName      string `json:"bankName"`
	Branch        string `json:"branch"`
	AccountType   string `json:"accountType"`
	BalancePaise  int64  `json:"balancePaise"`
	UPIID         string `json:"upiId"`
	HolderName    string `json:"holderName"`
}

type DummyBankResponse struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Account *DummyBankAccount `json:"account,omitempty"`
}

var dummyBankDB = map[string]*DummyBankAccount{}

func init() {
	now := time.Now().UTC()
	_ = now
	dummyBankDB = map[string]*DummyBankAccount{
		"jiyad@sbi": {
			AccountNumber: "12345678901", IFSC: "SBIN0001234", BankName: "State Bank of India",
			Branch: "MG Road, Bangalore", AccountType: "savings", BalancePaise: 35000000,
			UPIID: "jiyad@sbi", HolderName: "Jiyad",
		},
		"venkat@icici": {
			AccountNumber: "98765432109", IFSC: "ICIC0000456", BankName: "ICICI Bank",
			Branch: "Anna Salai, Chennai", AccountType: "savings", BalancePaise: 28000000,
			UPIID: "venkat@icici", HolderName: "Venkat",
		},
		"shubham@oksbi": {
			AccountNumber: "56789012345", IFSC: "KKBK0007789", BankName: "Kotak Mahindra Bank",
			Branch: "Bandra West, Mumbai", AccountType: "savings", BalancePaise: 60000000,
			UPIID: "shubham@oksbi", HolderName: "RD Shubham",
		},
		"electricity@hdfcbank": {
			AccountNumber: "11112222333", IFSC: "HDFC0004321", BankName: "HDFC Bank",
			Branch: "Corporate Banking", AccountType: "current", BalancePaise: 0,
			UPIID: "electricity@hdfcbank", HolderName: "BESCOM Electricity Board",
		},
		"bharatgas@indianoil": {
			AccountNumber: "22223333444", IFSC: "IOCL0000123", BankName: "Indian Oil Corporation",
			Branch: "LPG Division", AccountType: "current", BalancePaise: 0,
			UPIID: "bharatgas@indianoil", HolderName: "Bharat Gas Agency",
		},
		"amazon@paytm": {
			AccountNumber: "33334444555", IFSC: "PYTM0001234", BankName: "Paytm Payments Bank",
			Branch: "Noida", AccountType: "current", BalancePaise: 0,
			UPIID: "amazon@paytm", HolderName: "Amazon India",
		},
		"zerodha@hdfc": {
			AccountNumber: "44445555666", IFSC: "HDFC0000987", BankName: "HDFC Bank",
			Branch: "Brokerage Division", AccountType: "current", BalancePaise: 0,
			UPIID: "zerodha@hdfc", HolderName: "Zerodha Broking",
		},
		"school@pnb": {
			AccountNumber: "55556666777", IFSC: "PUNB0001234", BankName: "Punjab National Bank",
			Branch: "Education Division", AccountType: "current", BalancePaise: 0,
			UPIID: "school@pnb", HolderName: "Delhi Public School",
		},
	}
}

// The three DummyBank* methods below now flow through the BankAdapter seam
// (s.bank) when a provider is injected — the demo routes at
// /v1/dummy-bank/* are unchanged but resolve via the adapter. When no provider
// is set (e.g. a direct NewService() in a unit test) they fall back to the
// in-file legacy map so behaviour is identical either way.

func (s *Service) DummyBankVerifyUPI(upiID string) (DummyBankResponse, error) {
	upiID = strings.TrimSpace(strings.ToLower(upiID))
	if upiID == "" {
		return DummyBankResponse{}, fmt.Errorf("UPI ID is required")
	}
	if bank := s.bankAdapter(); bank != nil {
		status, holder, ok, err := bank.VerifyUPI(context.Background(), upiID)
		if err != nil {
			return DummyBankResponse{}, err
		}
		if ok {
			return DummyBankResponse{
				Status:  status,
				Message: fmt.Sprintf("UPI ID %s verified. Account holder: %s", upiID, holder),
				Account: dummyBankDB[upiID],
			}, nil
		}
		return DummyBankResponse{Status: firstNonEmpty(status, "not_found"),
			Message: fmt.Sprintf("UPI ID %s not found in bank records", upiID)}, nil
	}
	if acct, ok := dummyBankDB[upiID]; ok {
		return DummyBankResponse{
			Status:  "verified",
			Message: fmt.Sprintf("UPI ID %s verified. Account holder: %s, Bank: %s", upiID, acct.HolderName, acct.BankName),
			Account: acct,
		}, nil
	}
	return DummyBankResponse{
		Status:  "not_found",
		Message: fmt.Sprintf("UPI ID %s not found in bank records", upiID),
	}, nil
}

func (s *Service) DummyBankVerifyIFSC(ifsc, accountNumber string) (DummyBankResponse, error) {
	ifsc = strings.ToUpper(strings.TrimSpace(ifsc))
	accountNumber = strings.TrimSpace(accountNumber)
	if ifsc == "" || accountNumber == "" {
		return DummyBankResponse{}, fmt.Errorf("IFSC and account number are required")
	}
	if bank := s.bankAdapter(); bank != nil {
		status, holder, ok, err := bank.VerifyIFSC(context.Background(), ifsc, accountNumber)
		if err != nil {
			return DummyBankResponse{}, err
		}
		if ok {
			return DummyBankResponse{Status: status,
				Message: fmt.Sprintf("Account verified. Holder: %s", holder)}, nil
		}
		return DummyBankResponse{Status: firstNonEmpty(status, "not_found"),
			Message: fmt.Sprintf("No account found with IFSC %s and account number %s", ifsc, accountNumber)}, nil
	}
	for _, acct := range dummyBankDB {
		if acct.IFSC == ifsc && acct.AccountNumber == accountNumber {
			return DummyBankResponse{
				Status:  "verified",
				Message: fmt.Sprintf("Account verified. Holder: %s, Bank: %s", acct.HolderName, acct.BankName),
				Account: acct,
			}, nil
		}
	}
	return DummyBankResponse{
		Status:  "not_found",
		Message: fmt.Sprintf("No account found with IFSC %s and account number %s", ifsc, accountNumber),
	}, nil
}

func (s *Service) DummyBankBalance(accountID string) (DummyBankResponse, error) {
	if bank := s.bankAdapter(); bank != nil {
		paise, err := bank.GetBalance(context.Background(), accountID)
		if err != nil {
			return DummyBankResponse{}, err
		}
		return DummyBankResponse{
			Status:  "success",
			Message: fmt.Sprintf("Balance: %s", FormatMoney(paise)),
			Account: dummyBankDB[strings.ToLower(strings.TrimSpace(accountID))],
		}, nil
	}
	for _, acct := range dummyBankDB {
		if acct.AccountNumber == accountID || acct.UPIID == accountID {
			return DummyBankResponse{
				Status:  "success",
				Message: fmt.Sprintf("Balance for %s: %s", acct.HolderName, FormatMoney(acct.BalancePaise)),
				Account: acct,
			}, nil
		}
	}
	return DummyBankResponse{}, fmt.Errorf("account not found")
}
