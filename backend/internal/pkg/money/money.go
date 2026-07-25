package money

import "github.com/shopspring/decimal"

type INR struct {
	paise int64
}

func NewFromRupees(r decimal.Decimal) INR {
	return INR{paise: r.Mul(decimal.NewFromInt(100)).IntPart()}
}

func NewFromPaise(paise int64) INR {
	return INR{paise: paise}
}

func (m INR) Rupees() decimal.Decimal {
	return decimal.NewFromInt(m.paise).Div(decimal.NewFromInt(100))
}

func (m INR) Add(other INR) INR {
	return INR{paise: m.paise + other.paise}
}

func (m INR) Sub(other INR) INR {
	return INR{paise: m.paise - other.paise}
}

func (m INR) Paise() int64 {
	return m.paise
}

func (m INR) Format() string {
	return "INR " + m.Rupees().StringFixed(2)
}
