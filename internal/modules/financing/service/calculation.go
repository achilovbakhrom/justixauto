package service

import (
	"math/big"
	"slices"
	"time"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/money"
)

// The only calculation policy so far. It reproduces the documented
// illustrative "fixed markup" program of the reference prototype
// (business-logic §10): it is NOT an approved rule of any named bank or a
// legal qualification of murabaha; replace or re-approve before production.
const (
	PolicyFixedMarkup        = "fixed-markup"
	PolicyFixedMarkupVersion = 1
)

// halfUp returns round-half-up(n * bps / 10000) for non-negative n.
func halfUp(n *big.Int, bps int) *big.Int {
	num := new(big.Int).Mul(n, big.NewInt(int64(bps)))
	num.Add(num, big.NewInt(5_000))
	return num.Quo(num, big.NewInt(10_000))
}

// monthly returns the due date of payment i (0-based): the first due date's
// day in the following months, limited to the month's last day.
func monthly(first time.Time, i int) time.Time {
	y, m, d := first.Date()
	target := time.Date(y, m+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
	last := target.AddDate(0, 1, -1).Day()
	return time.Date(target.Year(), target.Month(), min(d, last), 0, 0, 0, 0, time.UTC)
}

// calculate applies the fixed-markup policy exactly (big integers, minor
// units): markup = half-up(price × markupBps / 10000); total = price + markup;
// financed = total − down; each payment = floor(financed / n) and the last
// one takes the remainder, so down + Σ payments = total and the final
// balance is zero.
func calculate(price money.Money, programID string, programVersion int, currency string, t model.ProgramTerms, e model.Eligibility, in model.CalculationInput, now time.Time) (*model.Calculation, error) {
	var v apperr.Validation
	p, ok := price.Parse()
	if !ok || p.Sign() == 0 || price.Currency != currency {
		return nil, apperr.New(apperr.ErrConflict, "currency_mismatch", "the sale price must be a positive amount in "+currency)
	}
	if e.MinPriceMinor != "" && p.Cmp(amount(e.MinPriceMinor)) < 0 || e.MaxPriceMinor != "" && p.Cmp(amount(e.MaxPriceMinor)) > 0 {
		return nil, apperr.New(apperr.ErrConflict, "price_not_eligible", "the sale price is outside the program limits")
	}
	down, ok := in.DownPayment.Parse()
	if !ok || in.DownPayment.Currency != currency {
		v.Add("calculationInputs.downPayment", "an amount in "+currency)
	}
	if !slices.Contains(t.TermMonths, in.TermMonths) {
		v.Add("calculationInputs.termMonths", "not a term of this program")
	}
	first, err := time.Parse(time.DateOnly, in.FirstDueDate)
	if err != nil {
		v.Add("calculationInputs.firstDueDate", "a date (YYYY-MM-DD)")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if minDown := halfUp(p, t.MinDownPaymentBps); down.Cmp(minDown) < 0 || down.Cmp(p) >= 0 {
		return nil, apperr.FieldError("calculationInputs.downPayment", "at least "+minDown.String()+" and below the price")
	}
	markup := halfUp(p, t.MarkupBps)
	total := new(big.Int).Add(p, markup)
	financed := new(big.Int).Sub(total, down)
	n := big.NewInt(int64(in.TermMonths))
	regular := new(big.Int).Quo(financed, n)
	c := &model.Calculation{
		Kind: "partner-program", PolicyID: PolicyFixedMarkup, PolicyVersion: PolicyFixedMarkupVersion,
		ProgramID: programID, ProgramVersion: programVersion, Price: price, Input: in, CalculatedAt: now,
	}
	c.Output.Markup, c.Output.Total, c.Output.FinancedAmount = money.Of(markup, currency), money.Of(total, currency), money.Of(financed, currency)
	balance := new(big.Int).Set(financed)
	for i := 0; i < in.TermMonths; i++ {
		pay := new(big.Int).Set(regular)
		if i == in.TermMonths-1 {
			pay.Set(balance)
		}
		balance.Sub(balance, pay)
		c.Output.Schedule = append(c.Output.Schedule, model.ScheduleRow{
			Number: i + 1, DueDate: monthly(first, i).Format(time.DateOnly),
			Total: money.Of(pay, currency), Balance: money.Of(new(big.Int).Set(balance), currency),
		})
	}
	return c, nil
}

func amount(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return new(big.Int)
	}
	return n
}
