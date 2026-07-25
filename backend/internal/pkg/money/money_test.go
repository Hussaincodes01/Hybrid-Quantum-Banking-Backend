package money

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestINRAdd(t *testing.T) {
	a := NewFromRupees(decimal.NewFromFloat(1000.25))
	b := NewFromRupees(decimal.NewFromFloat(250.25))
	got := a.Add(b)

	if got.Paise() != 125050 {
		t.Fatalf("expected 125050 paise, got %d", got.Paise())
	}

	if got.Format() != "INR 1250.50" {
		t.Fatalf("expected INR 1250.50, got %s", got.Format())
	}
}

func TestINRSub(t *testing.T) {
	a := NewFromPaise(9000)
	b := NewFromPaise(2500)
	got := a.Sub(b)

	if got.Paise() != 6500 {
		t.Fatalf("expected 6500 paise, got %d", got.Paise())
	}
}
