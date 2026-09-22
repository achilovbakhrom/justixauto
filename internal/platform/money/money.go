// Package money implements the contract's Money value: an integer amount in
// minor units (e.g. cents) as a decimal string, plus a currency code. Amounts
// are exact big integers; floating point is never used.
package money

import (
	"math/big"
	"regexp"
)

// Money is {amountMinor: "12345", currency: "USD"}.
type Money struct {
	AmountMinor string `json:"amountMinor"`
	Currency    string `json:"currency"`
}

var (
	amountPattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,37})$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Parse validates the value and returns the amount. ok is false if the amount
// is not a canonical non-negative integer of at most 38 digits or the currency
// is not a three-letter uppercase code.
func (m Money) Parse() (*big.Int, bool) {
	if !amountPattern.MatchString(m.AmountMinor) || !currencyPattern.MatchString(m.Currency) {
		return nil, false
	}
	n, ok := new(big.Int).SetString(m.AmountMinor, 10)
	return n, ok
}

// Of builds a Money from an exact amount.
func Of(amount *big.Int, currency string) Money {
	return Money{AmountMinor: amount.String(), Currency: currency}
}

// Fits reports whether an amount still has at most 38 digits.
func Fits(amount *big.Int) bool { return amount.Sign() >= 0 && len(amount.String()) <= 38 }
