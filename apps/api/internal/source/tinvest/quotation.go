package tinvest

import (
	"strconv"

	"github.com/shopspring/decimal"
)

// Quotation is Tinkoff's representation of a precise decimal: integer part
// (units) + 9-digit fractional (nano). Example: 100.5 = {units:"100", nano:500_000_000}.
type Quotation struct {
	Units string `json:"units"`
	Nano  int    `json:"nano"`
}

func (q Quotation) ToDecimal() decimal.Decimal {
	if q.Units == "" {
		return decimal.NewFromInt(int64(q.Nano)).Shift(-9)
	}
	units, err := strconv.ParseInt(q.Units, 10, 64)
	if err != nil {
		return decimal.Zero
	}
	intPart := decimal.NewFromInt(units)
	if q.Nano == 0 {
		return intPart
	}
	frac := decimal.NewFromInt(int64(q.Nano)).Shift(-9)
	if units < 0 || (units == 0 && q.Nano < 0) {
		// negative numbers — both fields are negative per Tinkoff spec
		return intPart.Add(frac)
	}
	return intPart.Add(frac)
}

// MoneyValue extends Quotation with a currency code (lowercase, e.g. "rub").
type MoneyValue struct {
	Currency string `json:"currency"`
	Units    string `json:"units"`
	Nano     int    `json:"nano"`
}

func (m MoneyValue) ToDecimal() decimal.Decimal {
	return Quotation{Units: m.Units, Nano: m.Nano}.ToDecimal()
}
