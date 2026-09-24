package service

import (
	"math/big"
	"testing"
	"time"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/money"
)

func usd(a string) money.Money { return money.Money{AmountMinor: a, Currency: "USD"} }

func TestFixedMarkupSchedule(t *testing.T) {
	terms := model.ProgramTerms{MarkupBps: 1000, MinDownPaymentBps: 2000, TermMonths: []int{7, 12}}
	c, err := calculate(usd("10000"), "p", 1, "USD", terms, model.Eligibility{}, model.CalculationInput{DownPayment: usd("2000"), TermMonths: 7, FirstDueDate: "2027-01-31"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.Output.Markup.AmountMinor != "1000" || c.Output.Total.AmountMinor != "11000" || c.Output.FinancedAmount.AmountMinor != "9000" {
		t.Fatalf("totals: %+v", c.Output)
	}
	sum := big.NewInt(2000)
	for _, r := range c.Output.Schedule {
		n, _ := new(big.Int).SetString(r.Total.AmountMinor, 10)
		sum.Add(sum, n)
	}
	rows := c.Output.Schedule
	if sum.String() != "11000" || rows[0].Total.AmountMinor != "1285" || rows[6].Total.AmountMinor != "1290" || rows[6].Balance.AmountMinor != "0" {
		t.Fatalf("schedule: %+v", rows)
	}
	// Day 31 is kept where possible and clamped to the month's end otherwise.
	if rows[0].DueDate != "2027-01-31" || rows[1].DueDate != "2027-02-28" || rows[2].DueDate != "2027-03-31" || rows[3].DueDate != "2027-04-30" {
		t.Fatalf("due dates: %v %v %v %v", rows[0].DueDate, rows[1].DueDate, rows[2].DueDate, rows[3].DueDate)
	}
}

func TestHalfUpAndLimits(t *testing.T) {
	if halfUp(big.NewInt(12345), 1000).String() != "1235" || halfUp(big.NewInt(12344), 1000).String() != "1234" {
		t.Fatal("half-up rounding")
	}
	terms := model.ProgramTerms{MarkupBps: 500, MinDownPaymentBps: 3000, TermMonths: []int{12}}
	in := model.CalculationInput{DownPayment: usd("2999"), TermMonths: 12, FirstDueDate: "2027-01-01"}
	if _, err := calculate(usd("10000"), "p", 1, "USD", terms, model.Eligibility{}, in, time.Now()); err == nil {
		t.Fatal("down payment below the minimum accepted")
	}
	in.DownPayment = usd("3000")
	if _, err := calculate(usd("10000"), "p", 1, "USD", terms, model.Eligibility{MaxPriceMinor: "9999"}, in, time.Now()); err == nil {
		t.Fatal("price above the program limit accepted")
	}
	if _, err := calculate(money.Money{AmountMinor: "10000", Currency: "EUR"}, "p", 1, "USD", terms, model.Eligibility{}, in, time.Now()); err == nil {
		t.Fatal("currency conversion must never happen")
	}
	in.TermMonths = 24
	if _, err := calculate(usd("10000"), "p", 1, "USD", terms, model.Eligibility{}, in, time.Now()); err == nil {
		t.Fatal("term outside the program accepted")
	}
}
