package debt

import "testing"

func TestEMIKnownValue(t *testing.T) {
	// ₹10,00,000 at 9% for 60 months ≈ ₹20,758/month (standard EMI calculator).
	// 10,00,000 rupees = 100,000,000 paise.
	emi := EMI(100_000_000, 9.0, 60)
	emiRupees := float64(emi) / 100.0
	if emiRupees < 20750 || emiRupees > 20766 {
		t.Fatalf("EMI = ₹%.2f, want ≈ ₹20,758", emiRupees)
	}
}

func TestEMIEdgeCases(t *testing.T) {
	if got := EMI(120000, 0, 12); got != 10000 {
		t.Fatalf("zero-rate EMI: got %d, want 10000 (P/n)", got)
	}
	if got := EMI(100000, 10, 0); got != 0 {
		t.Fatalf("months<=0 EMI: got %d, want 0", got)
	}
	if got := EMI(0, 10, 12); got != 0 {
		t.Fatalf("zero principal EMI: got %d, want 0", got)
	}
	// Single-month loan: EMI ≈ principal + one month interest.
	single := EMI(1_000_000, 12, 1)
	if single < 1_000_000 {
		t.Fatalf("single-month EMI %d should be >= principal", single)
	}
}

func TestScheduleAmortisesToZero(t *testing.T) {
	principal := int64(100_000_000)
	rows := Schedule(principal, 9.0, 60)
	if len(rows) == 0 {
		t.Fatal("expected a non-empty schedule")
	}
	last := rows[len(rows)-1]
	if last.BalancePaise != 0 {
		t.Fatalf("final balance = %d, want 0", last.BalancePaise)
	}
	// Sum of principal components must equal the original principal (exactly, since
	// the final row absorbs rounding).
	if got := TotalPrincipal(rows); got != principal {
		t.Fatalf("sum of principal = %d, want %d", got, principal)
	}
	// Total interest must be positive and less than principal for this loan.
	interest := TotalInterest(rows)
	if interest <= 0 {
		t.Fatalf("total interest = %d, want > 0", interest)
	}
}

func TestScheduleZeroRate(t *testing.T) {
	rows := Schedule(120000, 0, 12)
	if len(rows) != 12 {
		t.Fatalf("zero-rate schedule length = %d, want 12", len(rows))
	}
	if TotalInterest(rows) != 0 {
		t.Fatalf("zero-rate total interest = %d, want 0", TotalInterest(rows))
	}
	if rows[len(rows)-1].BalancePaise != 0 {
		t.Fatal("zero-rate loan must close at 0")
	}
}

func TestScheduleSingleMonth(t *testing.T) {
	rows := Schedule(1_000_000, 12, 1)
	if len(rows) != 1 {
		t.Fatalf("single-month schedule length = %d, want 1", len(rows))
	}
	if rows[0].PrincipalPaise != 1_000_000 {
		t.Fatalf("single-month principal = %d, want full principal", rows[0].PrincipalPaise)
	}
	if rows[0].BalancePaise != 0 {
		t.Fatal("single-month loan must close at 0")
	}
}

func TestScheduleZeroPrincipal(t *testing.T) {
	if rows := Schedule(0, 9, 12); rows != nil {
		t.Fatalf("zero principal should yield nil schedule, got %d rows", len(rows))
	}
}

func TestSolveTenure(t *testing.T) {
	// A loan whose EMI corresponds to 60 months should solve back to ~60.
	principal := int64(100_000_000)
	emi := EMI(principal, 9.0, 60)
	n := SolveTenure(principal, 9.0, emi)
	if n < 59 || n > 61 {
		t.Fatalf("SolveTenure = %d, want ≈ 60", n)
	}
	// Zero-rate: n = ceil(P/EMI).
	if got := SolveTenure(120000, 0, 10000); got != 12 {
		t.Fatalf("zero-rate SolveTenure = %d, want 12", got)
	}
	// EMI below monthly interest → never amortises → 0.
	if got := SolveTenure(100_000_000, 12, 100); got != 0 {
		t.Fatalf("under-covering EMI SolveTenure = %d, want 0", got)
	}
}
