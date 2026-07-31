package platform

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"FINIX/backend/internal/domain/security"
	"FINIX/backend/internal/domain/transaction"
)

type SeedUser struct {
	Name          string
	Mobile        string
	PIN           string
	Email         string
	NetWorthLakhs int64
	BankName      string
	IFSC          string
	AccountType   string
	BalancePaise  int64
	UPIID         string
	Goals         []SeedGoal
	Investments   []SeedInvestment
	Insurance     []SeedInsurance
	Loans         []SeedLoan
	Beneficiaries []SeedBeneficiary
	Transactions  []SeedTransaction
	PANLast4      string
	AadhaarLast4  string
	Occupation    string
	AnnualIncome  string
}

type SeedGoal struct {
	Name         string
	Description  string
	TargetPaise  int64
	SavedPaise   int64
	MonthlyPaise int64
	Priority     string
	TargetDate   time.Time
}

type SeedInvestment struct {
	Name       string
	Instrument string
	Category   string
	Units      float64
	AvgPrice   int64
	CurrentVal int64
	Invested   int64
	SIPAmount  int64
	SIPFreq    string
	Broker     string
	CAGR       float64
}

type SeedInsurance struct {
	PolicyType string
	Insurer    string
	SumAssured int64
	Premium    int64
}

type SeedLoan struct {
	Lender      string
	LoanType    string
	Outstanding int64
	EMI         int64
	Rate        float64
	Months      int
}

type SeedBeneficiary struct {
	Name       string
	UPIID      string
	TrustScore float64
}

type SeedTransaction struct {
	Recipient   string
	AmountPaise int64
	Channel     string
	DaysAgo     int
	Status      string
}

func seedUsers() []SeedUser {
	now := time.Now().UTC()
	return []SeedUser{
		{
			Name: "Jiyad", Mobile: "+919983692606", PIN: "123456", Email: "jiyad@finix.app",
			NetWorthLakhs: 85, BankName: "State Bank of India", IFSC: "SBIN0001234",
			AccountType: "savings", BalancePaise: 35000000, UPIID: "jiyad@sbi",
			PANLast4: "1234", AadhaarLast4: "1234", Occupation: "Software Engineer", AnnualIncome: "2400000",
			Goals: []SeedGoal{
				{Name: "House Down Payment", Description: "Save for 20% down payment on ₹1.2Cr apartment", TargetPaise: 24000000, SavedPaise: 12000000, MonthlyPaise: 1000000, Priority: "high", TargetDate: now.AddDate(2, 0, 0)},
				{Name: "Emergency Fund", Description: "6 months of expenses", TargetPaise: 7200000, SavedPaise: 4800000, MonthlyPaise: 200000, Priority: "high", TargetDate: now.AddDate(0, 6, 0)},
				{Name: "Europe Trip 2027", Description: "Family vacation fund", TargetPaise: 5000000, SavedPaise: 1500000, MonthlyPaise: 100000, Priority: "medium", TargetDate: now.AddDate(1, 6, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Nifty Index Fund", Instrument: "HDFC NIFTY 50 Index Fund", Category: "mutual_fund", Units: 185.5, AvgPrice: 28500, CurrentVal: 6800000, Invested: 5290000, SIPAmount: 2500000, SIPFreq: "monthly", Broker: "HDFC AMC", CAGR: 14.2},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 1500000, CurrentVal: 1900000, Invested: 1500000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "SBI", CAGR: 7.1},
				{Name: "Gold ETF", Instrument: "SBI Gold ETF", Category: "gold", Units: 75.0, AvgPrice: 15200, CurrentVal: 1450000, Invested: 1140000, SIPAmount: 0, SIPFreq: "", Broker: "SBI MF", CAGR: 9.5},
				{Name: "Axis Bluechip", Instrument: "Axis Bluechip Fund Direct Growth", Category: "mutual_fund", Units: 42000, AvgPrice: 58, CurrentVal: 3200000, Invested: 2440000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "Axis MF", CAGR: 15.8},
				{Name: "Corporate Bonds", Instrument: "HDFC Corporate Bond Fund", Category: "debt", Units: 25000, AvgPrice: 110, CurrentVal: 2800000, Invested: 2750000, SIPAmount: 0, SIPFreq: "", Broker: "HDFC MF", CAGR: 6.8},
				{Name: "Liquid Fund", Instrument: "ICICI Prudential Liquid Fund", Category: "liquid", Units: 50000, AvgPrice: 280, CurrentVal: 1420000, Invested: 1400000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "ICICI Pru", CAGR: 5.2},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "LIC", SumAssured: 100000000, Premium: 240000},
				{PolicyType: "health", Insurer: "Star Health", SumAssured: 15000000, Premium: 85000},
			},
			Loans: []SeedLoan{
				{Lender: "SBI", LoanType: "home", Outstanding: 320000000, EMI: 2850000, Rate: 8.55, Months: 212},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Venkat", UPIID: "venkat@icici", TrustScore: 0.92},
				{Name: "RD Shubham", UPIID: "shubham@oksbi", TrustScore: 0.95},
				{Name: "Electricity Board", UPIID: "electricity@hdfcbank", TrustScore: 0.80},
				{Name: "Mother", UPIID: "jiyadsmom@sbi", TrustScore: 0.99},
			},
			Transactions: []SeedTransaction{
				{Recipient: "venkat@icici", AmountPaise: 500000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "electricity@hdfcbank", AmountPaise: 420000, Channel: "upi", DaysAgo: 2, Status: "success"},
				{Recipient: "shubham@oksbi", AmountPaise: 2500000, Channel: "upi", DaysAgo: 3, Status: "success"},
				{Recipient: "amazon@paytm", AmountPaise: 899900, Channel: "upi", DaysAgo: 5, Status: "success"},
				{Recipient: "bigbazaar@upi", AmountPaise: 350000, Channel: "upi", DaysAgo: 7, Status: "success"},
				{Recipient: "unknown_urgent_99", AmountPaise: 5000000, Channel: "upi", DaysAgo: 4, Status: "blocked"},
			},
		},
		{
			Name: "Venkat", Mobile: "+916303891930", PIN: "123456", Email: "venkat@finix.app",
			NetWorthLakhs: 45, BankName: "ICICI Bank", IFSC: "ICIC0000456",
			AccountType: "savings", BalancePaise: 28000000, UPIID: "venkat@icici",
			PANLast4: "5678", AadhaarLast4: "5678", Occupation: "Business Analyst", AnnualIncome: "1800000",
			Goals: []SeedGoal{
				{Name: "Car Purchase", Description: "Save for Hyundai Creta", TargetPaise: 18000000, SavedPaise: 9000000, MonthlyPaise: 500000, Priority: "high", TargetDate: now.AddDate(1, 0, 0)},
				{Name: "Wedding Fund", Description: "Marriage expenses", TargetPaise: 15000000, SavedPaise: 4500000, MonthlyPaise: 300000, Priority: "medium", TargetDate: now.AddDate(1, 6, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Midcap Fund", Instrument: "Axis Midcap Fund Direct Growth", Category: "mutual_fund", Units: 38000, AvgPrice: 72, CurrentVal: 3200000, Invested: 2740000, SIPAmount: 1500000, SIPFreq: "monthly", Broker: "Axis MF", CAGR: 18.5},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 800000, CurrentVal: 950000, Invested: 800000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "ICICI", CAGR: 7.1},
				{Name: "Gold Sovereign Bonds", Instrument: "SGB 2024-25 Series", Category: "gold", Units: 30, AvgPrice: 74000, CurrentVal: 2400000, Invested: 2220000, SIPAmount: 0, SIPFreq: "", Broker: "RBI/SGB", CAGR: 8.2},
				{Name: "ELSS Tax Saver", Instrument: "Mirae Asset Tax Saver Fund", Category: "elss", Units: 22000, AvgPrice: 38, CurrentVal: 1050000, Invested: 836000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "Mirae Asset", CAGR: 16.2},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "HDFC Life", SumAssured: 50000000, Premium: 180000},
				{PolicyType: "health", Insurer: "New India Assurance", SumAssured: 10000000, Premium: 65000},
			},
			Loans: []SeedLoan{
				{Lender: "ICICI", LoanType: "car", Outstanding: 8500000, EMI: 185000, Rate: 9.50, Months: 48},
				{Lender: "Bajaj Finserv", LoanType: "personal", Outstanding: 3000000, EMI: 67000, Rate: 14.5, Months: 24},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Jiyad", UPIID: "jiyad@sbi", TrustScore: 0.92},
				{Name: "RD Shubham", UPIID: "shubham@oksbi", TrustScore: 0.95},
				{Name: "Bharat Gas", UPIID: "bharatgas@indianoil", TrustScore: 0.80},
			},
			Transactions: []SeedTransaction{
				{Recipient: "jiyad@sbi", AmountPaise: 300000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "bharatgas@indianoil", AmountPaise: 120000, Channel: "upi", DaysAgo: 2, Status: "success"},
				{Recipient: "shubham@oksbi", AmountPaise: 1500000, Channel: "upi", DaysAgo: 5, Status: "success"},
				{Recipient: "amazon@paytm", AmountPaise: 249900, Channel: "upi", DaysAgo: 6, Status: "success"},
				{Recipient: "rent@mygate", AmountPaise: 2200000, Channel: "upi", DaysAgo: 10, Status: "success"},
			},
		},
		{
			Name: "RD Shubham", Mobile: "+918175065652", PIN: "123456", Email: "shubham@finix.app",
			NetWorthLakhs: 120, BankName: "Kotak Mahindra Bank", IFSC: "KKBK0007789",
			AccountType: "savings", BalancePaise: 60000000, UPIID: "shubham@oksbi",
			PANLast4: "9012", AadhaarLast4: "9012", Occupation: "Investment Banker", AnnualIncome: "4500000",
			Goals: []SeedGoal{
				{Name: "Startup Capital", Description: "Seed funding for fintech startup", TargetPaise: 50000000, SavedPaise: 30000000, MonthlyPaise: 2000000, Priority: "high", TargetDate: now.AddDate(1, 0, 0)},
				{Name: "Retirement Corpus", Description: "Early retirement by 45", TargetPaise: 100000000, SavedPaise: 45000000, MonthlyPaise: 3000000, Priority: "high", TargetDate: now.AddDate(10, 0, 0)},
				{Name: "Dream Home", Description: "4BHK in Bandra", TargetPaise: 80000000, SavedPaise: 20000000, MonthlyPaise: 1500000, Priority: "medium", TargetDate: now.AddDate(5, 0, 0)},
				{Name: "Tesla Model 3", Description: "Electric vehicle fund", TargetPaise: 25000000, SavedPaise: 8000000, MonthlyPaise: 500000, Priority: "medium", TargetDate: now.AddDate(2, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Nifty 50 ETF", Instrument: "Nippon India Nifty ETF", Category: "mutual_fund", Units: 50000, AvgPrice: 220, CurrentVal: 14500000, Invested: 11000000, SIPAmount: 2000000, SIPFreq: "monthly", Broker: "Nippon India", CAGR: 16.5},
				{Name: "Small Cap Fund", Instrument: "SBI Small Cap Fund Direct Growth", Category: "mutual_fund", Units: 80000, AvgPrice: 95, CurrentVal: 12000000, Invested: 7600000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "SBI MF", CAGR: 22.8},
				{Name: "International Fund", Instrument: "Parag Parikh Flexi Cap (US Stocks)", Category: "mutual_fund", Units: 35000, AvgPrice: 75, CurrentVal: 4200000, Invested: 2630000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "PPFAS", CAGR: 19.2},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 2500000, CurrentVal: 2900000, Invested: 2500000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "Kotak", CAGR: 7.1},
				{Name: "Gold Bonds", Instrument: "Sovereign Gold Bonds 2023-24", Category: "gold", Units: 100, AvgPrice: 74000, CurrentVal: 8200000, Invested: 7400000, SIPAmount: 0, SIPFreq: "", Broker: "RBI/SGB", CAGR: 10.8},
				{Name: "Corporate FD", Instrument: "Shriram Transport FD", Category: "debt", Units: 1, AvgPrice: 5000000, CurrentVal: 5500000, Invested: 5000000, SIPAmount: 0, SIPFreq: "", Broker: "Shriram", CAGR: 8.5},
				{Name: "Liquid Fund", Instrument: "Kotak Liquid Fund", Category: "liquid", Units: 100000, AvgPrice: 320, CurrentVal: 3500000, Invested: 3200000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "Kotak MF", CAGR: 5.8},
				{Name: "Direct Stocks", Instrument: "Equity Direct (Demat)", Category: "equity", Units: 1, AvgPrice: 8000000, CurrentVal: 12500000, Invested: 8000000, SIPAmount: 0, SIPFreq: "", Broker: "Zerodha", CAGR: 24.5},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "Max Life", SumAssured: 200000000, Premium: 450000},
				{PolicyType: "health", Insurer: "Star Health", SumAssured: 25000000, Premium: 120000},
				{PolicyType: "car", Insurer: "Bajaj Allianz", SumAssured: 1500000, Premium: 35000},
			},
			Loans: []SeedLoan{
				{Lender: "Kotak", LoanType: "home", Outstanding: 450000000, EMI: 3800000, Rate: 8.20, Months: 180},
				{Lender: "HDFC", LoanType: "car", Outstanding: 12000000, EMI: 240000, Rate: 9.00, Months: 60},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Jiyad", UPIID: "jiyad@sbi", TrustScore: 0.92},
				{Name: "Venkat", UPIID: "venkat@icici", TrustScore: 0.92},
				{Name: "Brokerage", UPIID: "zerodha@hdfc", TrustScore: 0.85},
				{Name: "Mutual Fund", UPIID: "sip@kotak", TrustScore: 0.90},
				{Name: "Charity", UPIID: "give@cry", TrustScore: 0.75},
			},
			Transactions: []SeedTransaction{
				{Recipient: "jiyad@sbi", AmountPaise: 1000000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "zerodha@hdfc", AmountPaise: 5000000, Channel: "upi", DaysAgo: 2, Status: "success"},
				{Recipient: "sip@kotak", AmountPaise: 2000000, Channel: "upi", DaysAgo: 3, Status: "success"},
				{Recipient: "venkat@icici", AmountPaise: 1500000, Channel: "upi", DaysAgo: 4, Status: "success"},
				{Recipient: "give@cry", AmountPaise: 500000, Channel: "upi", DaysAgo: 7, Status: "success"},
				{Recipient: "rent@nobroker", AmountPaise: 8500000, Channel: "upi", DaysAgo: 10, Status: "success"},
				{Recipient: "electricity@hdfcbank", AmountPaise: 380000, Channel: "upi", DaysAgo: 12, Status: "success"},
			},
		},
		{
			Name: "Arjun Reddy", Mobile: "+919876543210", PIN: "123456", Email: "arjun@finix.app",
			NetWorthLakhs: 15, BankName: "Punjab National Bank", IFSC: "PUNB0001234",
			AccountType: "savings", BalancePaise: 12000000, UPIID: "arjun@pnb",
			PANLast4: "3456", AadhaarLast4: "3456", Occupation: "Teacher", AnnualIncome: "600000",
			Goals: []SeedGoal{
				{Name: "Daughter Education", Description: "College fund", TargetPaise: 10000000, SavedPaise: 3000000, MonthlyPaise: 100000, Priority: "high", TargetDate: now.AddDate(5, 0, 0)},
				{Name: "Home Renovation", Description: "House repair fund", TargetPaise: 5000000, SavedPaise: 1000000, MonthlyPaise: 50000, Priority: "medium", TargetDate: now.AddDate(2, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Balanced Fund", Instrument: "HDFC Balanced Advantage Fund", Category: "mutual_fund", Units: 30000, AvgPrice: 95, CurrentVal: 3500000, Invested: 2850000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "HDFC MF", CAGR: 11.2},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 500000, CurrentVal: 680000, Invested: 500000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "PNB", CAGR: 7.1},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "LIC", SumAssured: 30000000, Premium: 120000},
			},
			Loans: []SeedLoan{},
			Beneficiaries: []SeedBeneficiary{
				{Name: "School Fees", UPIID: "school@pnb", TrustScore: 0.85},
			},
			Transactions: []SeedTransaction{
				{Recipient: "school@pnb", AmountPaise: 250000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "groceries@bigbasket", AmountPaise: 350000, Channel: "upi", DaysAgo: 3, Status: "success"},
			},
		},
		{
			Name: "Priya Sharma", Mobile: "+919876543211", PIN: "123456", Email: "priya@finix.app",
			NetWorthLakhs: 55, BankName: "HDFC Bank", IFSC: "HDFC0004321",
			AccountType: "savings", BalancePaise: 40000000, UPIID: "priya@hdfc",
			PANLast4: "4567", AadhaarLast4: "4567", Occupation: "Doctor", AnnualIncome: "3200000",
			Goals: []SeedGoal{
				{Name: "Clinic Setup", Description: "Open private practice", TargetPaise: 30000000, SavedPaise: 15000000, MonthlyPaise: 500000, Priority: "high", TargetDate: now.AddDate(1, 0, 0)},
				{Name: "Child Education", Description: "Son's engineering", TargetPaise: 20000000, SavedPaise: 8000000, MonthlyPaise: 300000, Priority: "high", TargetDate: now.AddDate(8, 0, 0)},
				{Name: "Vacation Home", Description: "Goa property", TargetPaise: 15000000, SavedPaise: 3000000, MonthlyPaise: 150000, Priority: "low", TargetDate: now.AddDate(5, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Bluechip Fund", Instrument: "Mirae Asset Large Cap Fund", Category: "mutual_fund", Units: 60000, AvgPrice: 65, CurrentVal: 5200000, Invested: 3900000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "Mirae Asset", CAGR: 14.5},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 1800000, CurrentVal: 2200000, Invested: 1800000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "HDFC", CAGR: 7.1},
				{Name: "Gold ETF", Instrument: "Nippon Gold ETF", Category: "gold", Units: 40, AvgPrice: 15500, CurrentVal: 780000, Invested: 620000, SIPAmount: 0, SIPFreq: "", Broker: "Nippon", CAGR: 8.5},
				{Name: "Debt Fund", Instrument: "ICICI Corporate Bond Fund", Category: "debt", Units: 40000, AvgPrice: 105, CurrentVal: 4500000, Invested: 4200000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "ICICI Pru", CAGR: 6.8},
				{Name: "ELSS", Instrument: "Axis Tax Saver Fund", Category: "elss", Units: 25000, AvgPrice: 48, CurrentVal: 1550000, Invested: 1200000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "Axis MF", CAGR: 16.8},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "LIC", SumAssured: 75000000, Premium: 210000},
				{PolicyType: "health", Insurer: "Apollo Munich", SumAssured: 20000000, Premium: 95000},
			},
			Loans: []SeedLoan{
				{Lender: "HDFC", LoanType: "home", Outstanding: 280000000, EMI: 2500000, Rate: 8.40, Months: 200},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Hospital Supplier", UPIID: "medsupplies@hdfc", TrustScore: 0.82},
				{Name: "Jiyad", UPIID: "jiyad@sbi", TrustScore: 0.92},
			},
			Transactions: []SeedTransaction{
				{Recipient: "medsupplies@hdfc", AmountPaise: 850000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "jiyad@sbi", AmountPaise: 200000, Channel: "upi", DaysAgo: 4, Status: "success"},
				{Recipient: "electricity@hdfcbank", AmountPaise: 310000, Channel: "upi", DaysAgo: 5, Status: "success"},
			},
		},
		{
			Name: "Karthik Iyer", Mobile: "+919876543212", PIN: "123456", Email: "karthik@finix.app",
			NetWorthLakhs: 28, BankName: "AXIS Bank", IFSC: "UTIB0000987",
			AccountType: "savings", BalancePaise: 22000000, UPIID: "karthik@axis",
			PANLast4: "5678", AadhaarLast4: "5678", Occupation: "Marketing Manager", AnnualIncome: "1500000",
			Goals: []SeedGoal{
				{Name: "Bike Upgrade", Description: "Royal Enfield 650", TargetPaise: 4000000, SavedPaise: 2200000, MonthlyPaise: 100000, Priority: "medium", TargetDate: now.AddDate(0, 6, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Flexi Cap Fund", Instrument: "PPFAS Flexi Cap Fund", Category: "mutual_fund", Units: 28000, AvgPrice: 55, CurrentVal: 1850000, Invested: 1540000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "PPFAS", CAGR: 17.2},
				{Name: "Gold Sovereign", Instrument: "SGB 2024 Series", Category: "gold", Units: 15, AvgPrice: 74000, CurrentVal: 1250000, Invested: 1110000, SIPAmount: 0, SIPFreq: "", Broker: "RBI", CAGR: 8.5},
				{Name: "Liquid Fund", Instrument: "AXIS Liquid Fund", Category: "liquid", Units: 70000, AvgPrice: 280, CurrentVal: 2100000, Invested: 1960000, SIPAmount: 200000, SIPFreq: "monthly", Broker: "AXIS MF", CAGR: 5.5},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "Tata AIA", SumAssured: 40000000, Premium: 160000},
			},
			Loans: []SeedLoan{
				{Lender: "AXIS", LoanType: "personal", Outstanding: 5000000, EMI: 115000, Rate: 12.5, Months: 48},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Venkat", UPIID: "venkat@icici", TrustScore: 0.92},
			},
			Transactions: []SeedTransaction{
				{Recipient: "venkat@icici", AmountPaise: 150000, Channel: "upi", DaysAgo: 2, Status: "success"},
				{Recipient: "fuel@bpcl", AmountPaise: 50000, Channel: "upi", DaysAgo: 3, Status: "success"},
			},
		},
		{
			Name: "Sneha Patel", Mobile: "+919876543213", PIN: "123456", Email: "sneha@finix.app",
			NetWorthLakhs: 72, BankName: "Bank of India", IFSC: "BKID0005678",
			AccountType: "savings", BalancePaise: 45000000, UPIID: "sneha@boi",
			PANLast4: "6789", AadhaarLast4: "6789", Occupation: "Pharmacist", AnnualIncome: "2800000",
			Goals: []SeedGoal{
				{Name: "Pharmacy Chain", Description: "Expand to 5 stores", TargetPaise: 40000000, SavedPaise: 20000000, MonthlyPaise: 1000000, Priority: "high", TargetDate: now.AddDate(2, 0, 0)},
				{Name: "Son's MBA", Description: "IIM Ahmedabad fund", TargetPaise: 25000000, SavedPaise: 10000000, MonthlyPaise: 500000, Priority: "high", TargetDate: now.AddDate(3, 0, 0)},
				{Name: "Pilgrimage", Description: "Char Dham yatra", TargetPaise: 2000000, SavedPaise: 500000, MonthlyPaise: 50000, Priority: "low", TargetDate: now.AddDate(1, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Large Cap Fund", Instrument: "SBI Bluechip Fund", Category: "mutual_fund", Units: 50000, AvgPrice: 85, CurrentVal: 5200000, Invested: 4250000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "SBI MF", CAGR: 14.8},
				{Name: "Mid Cap", Instrument: "Kotak Emerging Equity", Category: "mutual_fund", Units: 35000, AvgPrice: 65, CurrentVal: 3100000, Invested: 2275000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "Kotak MF", CAGR: 19.5},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 2000000, CurrentVal: 2500000, Invested: 2000000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "BOI", CAGR: 7.1},
				{Name: "Gold Bonds", Instrument: "SGB Series", Category: "gold", Units: 50, AvgPrice: 73000, CurrentVal: 4100000, Invested: 3650000, SIPAmount: 0, SIPFreq: "", Broker: "RBI", CAGR: 9.2},
				{Name: "Debt Fund", Instrument: "HDFC Short Term Debt", Category: "debt", Units: 30000, AvgPrice: 115, CurrentVal: 3800000, Invested: 3450000, SIPAmount: 300000, SIPFreq: "monthly", Broker: "HDFC MF", CAGR: 6.5},
				{Name: "International", Instrument: "Nippon India US Equity", Category: "mutual_fund", Units: 20000, AvgPrice: 95, CurrentVal: 2200000, Invested: 1900000, SIPAmount: 300000, SIPFreq: "monthly", Broker: "Nippon", CAGR: 15.2},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "LIC", SumAssured: 80000000, Premium: 220000},
				{PolicyType: "health", Insurer: "New India Assurance", SumAssured: 15000000, Premium: 78000},
			},
			Loans: []SeedLoan{
				{Lender: "BOI", LoanType: "business", Outstanding: 150000000, EMI: 1350000, Rate: 10.5, Months: 180},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Med Supplier", UPIID: "medsupplier@boi", TrustScore: 0.88},
				{Name: "RD Shubham", UPIID: "shubham@oksbi", TrustScore: 0.95},
			},
			Transactions: []SeedTransaction{
				{Recipient: "medsupplier@boi", AmountPaise: 1200000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "shubham@oksbi", AmountPaise: 800000, Channel: "upi", DaysAgo: 3, Status: "success"},
			},
		},
		{
			Name: "Ravi Kumar", Mobile: "+919876543214", PIN: "123456", Email: "ravi@finix.app",
			NetWorthLakhs: 8, BankName: "Canara Bank", IFSC: "CNRB0003456",
			AccountType: "savings", BalancePaise: 6000000, UPIID: "ravi@canara",
			PANLast4: "7890", AadhaarLast4: "7890", Occupation: "Graduate Student", AnnualIncome: "350000",
			Goals: []SeedGoal{
				{Name: "First Car", Description: "Second-hand Swift", TargetPaise: 4000000, SavedPaise: 1500000, MonthlyPaise: 50000, Priority: "medium", TargetDate: now.AddDate(2, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Index Fund", Instrument: "UTI Nifty 50 Index Fund", Category: "mutual_fund", Units: 15000, AvgPrice: 130, CurrentVal: 2200000, Invested: 1950000, SIPAmount: 100000, SIPFreq: "monthly", Broker: "UTI MF", CAGR: 12.5},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "health", Insurer: "Star Health", SumAssured: 5000000, Premium: 25000},
			},
			Loans: []SeedLoan{
				{Lender: "Canara", LoanType: "education", Outstanding: 3500000, EMI: 35000, Rate: 8.5, Months: 120},
			},
			Beneficiaries: []SeedBeneficiary{},
			Transactions: []SeedTransaction{
				{Recipient: "college@canara", AmountPaise: 50000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "mess@zomato", AmountPaise: 45000, Channel: "upi", DaysAgo: 2, Status: "success"},
			},
		},
		{
			Name: "Ananya Gupta", Mobile: "+919876543215", PIN: "123456", Email: "ananya@finix.app",
			NetWorthLakhs: 95, BankName: "ICICI Bank", IFSC: "ICIC0000789",
			AccountType: "savings", BalancePaise: 55000000, UPIID: "ananya@icici",
			PANLast4: "8901", AadhaarLast4: "8901", Occupation: "Tech Entrepreneur", AnnualIncome: "6000000",
			Goals: []SeedGoal{
				{Name: "Series A Fund", Description: "Bootstrap to Series A", TargetPaise: 80000000, SavedPaise: 50000000, MonthlyPaise: 3000000, Priority: "high", TargetDate: now.AddDate(1, 0, 0)},
				{Name: "Luxury Apartment", Description: "3BHK in Powai", TargetPaise: 60000000, SavedPaise: 25000000, MonthlyPaise: 2000000, Priority: "high", TargetDate: now.AddDate(3, 0, 0)},
				{Name: "World Tour", Description: "Europe + Japan", TargetPaise: 8000000, SavedPaise: 2000000, MonthlyPaise: 200000, Priority: "medium", TargetDate: now.AddDate(2, 0, 0)},
				{Name: "Angel Investing", Description: "Startup portfolio", TargetPaise: 20000000, SavedPaise: 5000000, MonthlyPaise: 500000, Priority: "low", TargetDate: now.AddDate(5, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Tech Fund", Instrument: "ICICI Technology Fund", Category: "mutual_fund", Units: 40000, AvgPrice: 95, CurrentVal: 6800000, Invested: 3800000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "ICICI Pru", CAGR: 25.5},
				{Name: "Nasdaq 100 FoF", Instrument: "Motilal Nasdaq 100 FoF", Category: "mutual_fund", Units: 30000, AvgPrice: 115, CurrentVal: 5200000, Invested: 3450000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "Motilal Oswal", CAGR: 22.8},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 2500000, CurrentVal: 3100000, Invested: 2500000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "ICICI", CAGR: 7.1},
				{Name: "Gold Bonds", Instrument: "SGB Series", Category: "gold", Units: 80, AvgPrice: 72000, CurrentVal: 6500000, Invested: 5760000, SIPAmount: 0, SIPFreq: "", Broker: "RBI", CAGR: 10.5},
				{Name: "Debt", Instrument: "HDFC Corporate Bond", Category: "debt", Units: 50000, AvgPrice: 110, CurrentVal: 5800000, Invested: 5500000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "HDFC MF", CAGR: 6.8},
				{Name: "Liquid", Instrument: "ICICI Liquid Fund", Category: "liquid", Units: 150000, AvgPrice: 290, CurrentVal: 4800000, Invested: 4350000, SIPAmount: 1000000, SIPFreq: "monthly", Broker: "ICICI Pru", CAGR: 5.5},
				{Name: "Direct Equity", Instrument: "Stocks (Demat)", Category: "equity", Units: 1, AvgPrice: 5000000, CurrentVal: 8500000, Invested: 5000000, SIPAmount: 0, SIPFreq: "", Broker: "Zerodha", CAGR: 28.2},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "Max Life", SumAssured: 150000000, Premium: 380000},
				{PolicyType: "health", Insurer: "Cigna TTK", SumAssured: 30000000, Premium: 140000},
			},
			Loans: []SeedLoan{},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Co-founder", UPIID: "cofounder@icici", TrustScore: 0.95},
				{Name: "RD Shubham", UPIID: "shubham@oksbi", TrustScore: 0.95},
			},
			Transactions: []SeedTransaction{
				{Recipient: "cofounder@icici", AmountPaise: 2000000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "shubham@oksbi", AmountPaise: 3000000, Channel: "upi", DaysAgo: 2, Status: "success"},
				{Recipient: "aws@amazon", AmountPaise: 850000, Channel: "upi", DaysAgo: 5, Status: "success"},
			},
		},
		{
			Name: "Mohammed Ali", Mobile: "+919876543216", PIN: "123456", Email: "ali@finix.app",
			NetWorthLakhs: 38, BankName: "Union Bank of India", IFSC: "UBIN0005678",
			AccountType: "savings", BalancePaise: 30000000, UPIID: "ali@unionbank",
			PANLast4: "9012", AadhaarLast4: "9012", Occupation: "Government Employee", AnnualIncome: "1200000",
			Goals: []SeedGoal{
				{Name: "Children Education", Description: "Two kids college", TargetPaise: 15000000, SavedPaise: 6000000, MonthlyPaise: 200000, Priority: "high", TargetDate: now.AddDate(6, 0, 0)},
				{Name: "Hajj Pilgrimage", Description: "Family pilgrimage", TargetPaise: 8000000, SavedPaise: 2000000, MonthlyPaise: 100000, Priority: "medium", TargetDate: now.AddDate(3, 0, 0)},
			},
			Investments: []SeedInvestment{
				{Name: "Balanced Fund", Instrument: "HDFC Balanced Fund", Category: "mutual_fund", Units: 35000, AvgPrice: 78, CurrentVal: 3500000, Invested: 2730000, SIPAmount: 500000, SIPFreq: "monthly", Broker: "HDFC MF", CAGR: 12.8},
				{Name: "PPF", Instrument: "Public Provident Fund", Category: "ppf", Units: 1, AvgPrice: 1200000, CurrentVal: 1500000, Invested: 1200000, SIPAmount: 150000, SIPFreq: "yearly", Broker: "Union Bank", CAGR: 7.1},
				{Name: "Gold", Instrument: "SGB Series", Category: "gold", Units: 25, AvgPrice: 73000, CurrentVal: 2100000, Invested: 1825000, SIPAmount: 0, SIPFreq: "", Broker: "RBI", CAGR: 8.8},
			},
			Insurance: []SeedInsurance{
				{PolicyType: "term_life", Insurer: "LIC", SumAssured: 50000000, Premium: 180000},
				{PolicyType: "health", Insurer: "New India Assurance", SumAssured: 10000000, Premium: 55000},
			},
			Loans: []SeedLoan{
				{Lender: "Union Bank", LoanType: "home", Outstanding: 180000000, EMI: 1600000, Rate: 8.30, Months: 200},
			},
			Beneficiaries: []SeedBeneficiary{
				{Name: "Jiyad", UPIID: "jiyad@sbi", TrustScore: 0.92},
			},
			Transactions: []SeedTransaction{
				{Recipient: "jiyad@sbi", AmountPaise: 100000, Channel: "upi", DaysAgo: 1, Status: "success"},
				{Recipient: "electricity@unionbank", AmountPaise: 280000, Channel: "upi", DaysAgo: 3, Status: "success"},
				{Recipient: "groceries@bigbasket", AmountPaise: 180000, Channel: "upi", DaysAgo: 5, Status: "success"},
			},
		},
	}
}

func (s *Service) SeedDemoUsers() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	users := seedUsers()
	for _, su := range users {
		if _, exists := s.mobileIndex[su.Mobile]; exists {
			continue
		}

		userID := "usr_" + randomHex(16)
		ubt := "ubt_" + randomHex(10)
		now := time.Now().UTC()

		simHash := security.HashSIMICCID(su.Mobile + ":device-demo-fp")
		simBound := false
		if ch, err := s.simBinding.CreateBindingChallenge(userID, su.Mobile, "device-demo-fp"); err == nil {
			if record, err := s.simBinding.VerifyBindingChallenge(ch.ChallengeID, ch.VNNCode, simHash); err == nil {
				ubt = record.UBT
				simBound = true
			}
		}

		user := &User{
			ID: userID, Name: su.Name, Mobile: su.Mobile, Email: su.Email,
			UBT: ubt, EKYCVerified: true, BiometricEnabled: true,
			DeviceBound: true, SIMBound: simBound, NudgePreference: "moderate",
			KIN: security.GenerateKIN(su.AadhaarLast4, su.PANLast4), CreatedAt: now,
		}
		s.users[userID] = user
		s.mobileIndex[su.Mobile] = userID

		pinHash, _ := HashPIN(su.PIN)

		s.authProfiles[userID] = &AuthProfile{
			InternalUserID: userID, FullName: su.Name,
			MobileNumber: EncryptField(su.Mobile), Email: EncryptField(su.Email),
			DeviceIDFingerprint: "device-demo-fp", DeviceType: "android",
			AppVersion: "1.0.0", IPAddress: "127.0.0.1",
			TrustedDeviceFlag: true, ChallengeStatus: "verified",
			StepUpAuthRequiredFlag: false, Role: "customer",
			PasswordHash: pinHash,
		}

		s.kycProfiles[userID] = &KYCProfile{
			UserID: userID, FullName: su.Name, MobileNumber: su.Mobile, Email: su.Email,
			PANMasked: MaskPAN("XXXXXX" + su.PANLast4), AadhaarMaskedOrHash: "XXXXXXXX" + su.AadhaarLast4,
			KYCStatus: "verified", KYCSubmissionDate: &now, KYCVerificationMethod: "offline_xml+nsdl",
			Occupation: su.Occupation, AnnualIncome: su.AnnualIncome, TaxResidency: "new",
		}

		acctID := "acct_" + randomHex(8)
		accountToken := TokenizeAccount(acctID, su.Mobile+":"+su.IFSC)
		s.accounts[userID] = []Account{{
			ID: acctID, AccountHolderName: su.Name, BankName: su.BankName,
			Branch: "Main Branch", IFSCCode: su.IFSC,
			AccountNumberMaskedToken: accountToken, UPIID: su.UPIID,
			AccountType: su.AccountType, VerificationStatus: "verified",
			Nickname: "Primary " + su.AccountType, PrimaryAccountFlag: true,
			AccountToken: "accttok_" + randomHex(8), TokenProvider: "bank",
			AccountVerificationAt: &now, AccountVerificationMethod: "upi",
			BalancePaise: su.BalancePaise, LinkedAt: now,
		}}

		s.beneficiaries[userID] = make(map[string]struct{})
		s.beneficiaryRecords[userID] = []Beneficiary{}
		for _, b := range su.Beneficiaries {
			bID := "benf_" + randomHex(6)
			s.beneficiaryRecords[userID] = append(s.beneficiaryRecords[userID], Beneficiary{
				ID: bID, BeneficiaryName: b.Name, UPIIDOrBankDetails: b.UPIID,
				TrustScore: b.TrustScore, DateAdded: now, AddedBy: userID,
				VerificationStatus: "verified", RelationshipDescription: "contact",
			})
			s.beneficiaries[userID][strings.ToLower(b.UPIID)] = struct{}{}
		}

		s.aggregatorStatus[userID] = AggregatorStatus{
			LinkedInstitutionID: su.IFSC[:4], ConnectorStatus: "connected",
			LastSyncTimestamp: &now, NumberOfAccountsAggregated: 1,
		}

		s.consents[userID] = map[string]bool{
			"analytics": true, "market_data": true, "notifications": true,
			"sms_scanning": false, "model_improvement": true, "privacy_policy": true,
		}

		s.notifications[userID] = []NotificationSetting{
			{Category: "transactions", Enabled: true},
			{Category: "security", Enabled: true},
			{Category: "goals", Enabled: true},
			{Category: "insights", Enabled: true},
			{Category: "tax", Enabled: true},
		}

		for _, g := range su.Goals {
			goal := Goal{
				ID: "goal_" + randomHex(6), Name: g.Name, Description: g.Description,
				TargetAmountPaise: g.TargetPaise, Currency: "INR",
				SavedAmountPaise: g.SavedPaise, MonthlyContributionPaise: g.MonthlyPaise,
				Frequency: "monthly", Priority: g.Priority, StartDate: now.AddDate(-1, 0, 0),
				TargetDate: g.TargetDate, Status: "active", LinkedAccount: acctID,
				CreatedAt: now.AddDate(-1, 0, 0),
			}
			computeGoalProgress(&goal)
			s.goals[userID] = append(s.goals[userID], goal)
		}

		for i, inv := range su.Investments {
			s.investments[userID] = append(s.investments[userID], InvestmentHolding{
				ID:   fmt.Sprintf("hold_%s_%02d", userID[:8], i+1),
				Name: inv.Name, InstrumentName: inv.Instrument, Category: inv.Category,
				QuantityUnits: inv.Units, AvgPurchasePricePaise: inv.AvgPrice,
				CurrentValuePaise: inv.CurrentVal, InvestedPaise: inv.Invested,
				LastValuationDate: now, SIPAmountPaise: inv.SIPAmount,
				SIPFrequency: inv.SIPFreq, SIPStartDate: now.AddDate(-1, 0, 0),
				SIPStatus: "active", BrokerFundHouse: inv.Broker,
				DividendsPaise: 0, CAGR: inv.CAGR, CreatedAt: now.AddDate(-1, 0, 0),
			})
		}

		for _, ins := range su.Insurance {
			s.insurance[userID] = append(s.insurance[userID], InsurancePolicy{
				PolicyID: "pol_" + randomHex(6), PolicyType: ins.PolicyType,
				Insurer: ins.Insurer, SumAssuredPaise: ins.SumAssured,
				PremiumPaise: ins.Premium, NextDueDate: now.AddDate(0, 1, 0).Format("2006-01-02"),
			})
		}

		for _, ln := range su.Loans {
			s.loans[userID] = append(s.loans[userID], LoanRecord{
				LoanID: "loan_" + randomHex(6), Lender: ln.Lender, LoanType: ln.LoanType,
				OutstandingPaise: ln.Outstanding, EMIPaise: ln.EMI,
				InterestRate: ln.Rate, RemainingMonths: ln.Months,
			})
		}

		// Top up the primary account balance so the net worth reported by
		// /v1/portfolio/net-worth matches the intended NetWorthLakhs. Some seeds
		// carry large home loans that dwarf the modest seeded balance, which would
		// otherwise yield a negative net worth.
		//
		// IMPORTANT: this MUST mirror NetWorthSnapshot's asset definition exactly —
		// it counts bank balances + investment current value as assets and loans as
		// liabilities, and does NOT count insurance. Previously this top-up also
		// credited insurance/20, so it under-funded the balance and the endpoint
		// reported a net worth below the target (and negative for low-target,
		// high-insurance users). Excluding insurance here makes the endpoint report
		// exactly NetWorthLakhs.
		{
			targetNetWorthPaise := int64(su.NetWorthLakhs) * 100000 * 100
			var investmentValue int64
			for _, inv := range s.investments[userID] {
				investmentValue += inv.CurrentValuePaise
			}
			var liabilities int64
			for _, ln := range s.loans[userID] {
				liabilities += ln.OutstandingPaise
			}
			// balance so that balance + investments - liabilities == target.
			neededBalance := targetNetWorthPaise + liabilities - investmentValue
			if neededBalance > s.accounts[userID][0].BalancePaise {
				s.accounts[userID][0].BalancePaise = neededBalance
			}
			// Safety net: never surface a net worth below the target.
			actualNW := s.accounts[userID][0].BalancePaise + investmentValue - liabilities
			if actualNW < targetNetWorthPaise {
				s.accounts[userID][0].BalancePaise += targetNetWorthPaise - actualNW
			}
		}

		for _, tx := range su.Transactions {
			txTime := now.AddDate(0, 0, -tx.DaysAgo)
			riskLevel := transaction.RiskLow
			riskScore := 15.0
			if tx.Status == "blocked" {
				riskLevel = transaction.RiskHigh
				riskScore = 85.0
			} else if tx.AmountPaise > 1000000 {
				riskLevel = transaction.RiskMedium
				riskScore = 45.0
			}
			txn := Transaction{
				ID: "txn_" + randomHex(8), UserID: userID, AmountPaise: tx.AmountPaise,
				Currency: "INR", DebitCredit: "debit", Recipient: tx.Recipient,
				MerchantName: tx.Recipient, Channel: tx.Channel,
				Description: fmt.Sprintf("Payment to %s", tx.Recipient),
				Status:      tx.Status, RiskLevel: riskLevel, RiskScore: riskScore,
				XAIReason: "Transaction within normal pattern.", LinkedAccount: acctID,
				CreatedAt: txTime, IdempotencyKey: "idem_" + randomHex(8),
				TimelineIDs: []string{},
			}
			s.transactions[userID] = append(s.transactions[userID], txn)
			if tx.Status == "success" {
				s.accountingLedger.PostOutgoingPayment(txn.ID, acctID, userID, tx.Recipient, tx.AmountPaise, txn.Description)
				s.fraudGraph.AddTransaction(userID, tx.Recipient, tx.AmountPaise)
			}
		}

		s.appendAuditLocked(userID, "registration_completed", "system", "success", "Demo user seeded with full portfolio", "System")

		if s.db != nil {
			db := s.db
			savedUser := *user
			savedAcct := s.accounts[userID][0]
			savedConsents := make(map[string]bool)
			for k, v := range s.consents[userID] {
				savedConsents[k] = v
			}
			go func() {
				ctx := context.Background()
				_ = db.SaveUserToDB(ctx, savedUser)
				_ = db.SaveAccountToDB(ctx, savedAcct, userID)
				for ct, g := range savedConsents {
					_ = db.SaveConsentToDB(ctx, userID, ct, g)
				}
			}()
		}
	}

	log.Printf("[SEED] Seeded %d demo users with full portfolios", len(users))
	return nil
}

func (s *Service) NeedsSeeding() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users) == 0
}
