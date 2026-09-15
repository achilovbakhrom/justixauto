// Package money provides immutable, fixed-precision monetary values.
//
// A Money value is always tied to the exact version of the currency exponent
// used when its minor-unit amount was recorded. The exponent is metadata; the
// amount is never rescaled and this package never performs foreign exchange.
package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
)

const MaxAmountDigits = 38

var (
	ErrInvalidAmount       = errors.New("money: invalid amountMinor")
	ErrInvalidCurrency     = errors.New("money: invalid currency definition")
	ErrCurrencyMismatch    = errors.New("money: currency definition mismatch")
	ErrAmountOverflow      = errors.New("money: amount exceeds 38 digits")
	currencyCodePattern    = regexp.MustCompile(`^[A-Z]+$`)
	canonicalIntegerRegexp = regexp.MustCompile(`^(0|-[1-9][0-9]*|[1-9][0-9]*)$`)
)

// Currency identifies one version of a currency's minor-unit definition.
// Version is opaque and must be supplied by an approved currency dictionary.
type Currency struct {
	code            string
	exponent        uint8
	exponentVersion string
}

func NewCurrency(code string, exponent uint8, exponentVersion string) (Currency, error) {
	if !currencyCodePattern.MatchString(code) || strings.TrimSpace(exponentVersion) == "" || exponentVersion != strings.TrimSpace(exponentVersion) {
		return Currency{}, ErrInvalidCurrency
	}
	return Currency{code: code, exponent: exponent, exponentVersion: exponentVersion}, nil
}

func (c Currency) Code() string            { return c.code }
func (c Currency) Exponent() uint8         { return c.exponent }
func (c Currency) ExponentVersion() string { return c.exponentVersion }
func (c Currency) valid() bool {
	return currencyCodePattern.MatchString(c.code) && c.exponentVersion != ""
}
func (c Currency) same(other Currency) bool { return c == other }

// Money stores a canonical signed integer number of minor units. Its internal
// big.Int is never exposed, so callers cannot mutate a value after creation.
type Money struct {
	amount   *big.Int
	currency Currency
}

func New(amountMinor string, currency Currency) (Money, error) {
	amount, err := parseAmount(amountMinor)
	if err != nil {
		return Money{}, err
	}
	if !currency.valid() {
		return Money{}, ErrInvalidCurrency
	}
	return Money{amount: amount, currency: currency}, nil
}

func parseAmount(value string) (*big.Int, error) {
	if !canonicalIntegerRegexp.MatchString(value) {
		return nil, ErrInvalidAmount
	}
	digits := value
	if digits[0] == '-' {
		digits = digits[1:]
	}
	if len(digits) > MaxAmountDigits {
		return nil, ErrAmountOverflow
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, ErrInvalidAmount
	}
	return amount, nil
}

func (m Money) valid() bool { return m.amount != nil && m.currency.valid() }

func (m Money) AmountMinor() string {
	if m.amount == nil {
		return ""
	}
	return m.amount.String()
}

func (m Money) Currency() Currency { return m.currency }
func (m Money) IsZero() bool       { return m.amount != nil && m.amount.Sign() == 0 }

func (m Money) Equal(other Money) bool {
	return m.valid() && other.valid() && m.currency.same(other.currency) && m.amount.Cmp(other.amount) == 0
}

func (m Money) Add(other Money) (Money, error) { return m.combine(other, new(big.Int).Add) }
func (m Money) Sub(other Money) (Money, error) { return m.combine(other, new(big.Int).Sub) }

func (m Money) combine(other Money, operation func(*big.Int, *big.Int) *big.Int) (Money, error) {
	if !m.valid() || !other.valid() {
		return Money{}, ErrInvalidAmount
	}
	if !m.currency.same(other.currency) {
		return Money{}, ErrCurrencyMismatch
	}
	result := operation(new(big.Int).Set(m.amount), other.amount)
	return New(result.String(), m.currency)
}

type wireMoney struct {
	AmountMinor string `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// MarshalJSON implements the approved public Money contract. Exponent metadata
// remains part of the typed value and is not silently added to that wire shape.
func (m Money) MarshalJSON() ([]byte, error) {
	if !m.valid() {
		return nil, errors.New("money: cannot marshal invalid value")
	}
	return json.Marshal(wireMoney{AmountMinor: m.AmountMinor(), Currency: m.currency.code})
}

// ParseJSON validates the exact public JSON shape against a caller-selected,
// versioned currency definition. Currency metadata can therefore never be
// guessed from the currency code in an old payload.
func ParseJSON(data []byte, currency Currency) (Money, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return Money{}, errors.New("money: expected JSON object")
	}
	values := map[string]string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Money{}, fmt.Errorf("money: decode field: %w", err)
		}
		name, ok := token.(string)
		if !ok || (name != "amountMinor" && name != "currency") {
			return Money{}, fmt.Errorf("money: unknown field %q", token)
		}
		if _, duplicate := values[name]; duplicate {
			return Money{}, fmt.Errorf("money: duplicate field %q", name)
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return Money{}, fmt.Errorf("money: field %s must be a string: %w", name, err)
		}
		values[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return Money{}, errors.New("money: incomplete JSON object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Money{}, errors.New("money: trailing JSON content")
	}
	amountMinor, amountPresent := values["amountMinor"]
	wireCurrency, currencyPresent := values["currency"]
	if !amountPresent || !currencyPresent {
		return Money{}, errors.New("money: amountMinor and currency are required")
	}
	if !currency.valid() || wireCurrency != currency.code {
		return Money{}, ErrCurrencyMismatch
	}
	return New(amountMinor, currency)
}
