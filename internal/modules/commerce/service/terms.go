package service

import (
	"context"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

// validateTerms normalizes terms and returns the exact total. All amounts
// share one currency (no conversion); a payment schedule must add up to the
// total exactly. Line and installment IDs are generated when missing.
func (d Deps) validateTerms(ctx context.Context, v *apperr.Validation, in model.Terms) model.Terms {
	out := model.Terms{
		Route: in.Route, Lines: []model.Line{}, PaymentSchedule: []model.Installment{},
		DeliveryTerms: validate.Text(v, "terms.deliveryTerms", in.DeliveryTerms, 0, 2000),
		WarrantyTerms: validate.Text(v, "terms.warrantyTerms", in.WarrantyTerms, 0, 2000),
		ServiceTerms:  validate.Text(v, "terms.serviceTerms", in.ServiceTerms, 0, 2000),
	}
	if !slices.Contains(model.Routes, in.Route) {
		v.Add("terms.route", "must be factory, foreign-direct, in-transit or local")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		v.Add("terms.lines", "list 1-100 lines")
	}
	currency := ""
	ids := map[string]bool{}
	total := d.validateLines(ctx, v, in.Lines, ids, &out.Lines, &currency)
	if !money.Fits(total) {
		v.Add("terms.lines", "total is too large")
	}
	if len(in.PaymentSchedule) > 60 {
		v.Add("terms.paymentSchedule", "at most 60 installments")
	}
	scheduled := validateSchedule(v, in.PaymentSchedule, ids, &out.PaymentSchedule, &currency)
	if len(in.PaymentSchedule) > 0 && scheduled.Cmp(total) != 0 {
		v.Add("terms.paymentSchedule", "installments must add up to the total "+total.String())
	}
	return out
}

// sameCurrency records the first currency seen and flags any later amount
// that uses a different one.
func sameCurrency(v *apperr.Validation, currency *string, field, c string) {
	if *currency == "" {
		*currency = c
	} else if c != *currency {
		v.Add(field, "all amounts must use one currency ("+*currency+")")
	}
}

// validateLines normalizes order lines, checks each against the catalog and
// returns the exact total (unit price * quantity, summed). Generated or
// duplicate line IDs are tracked in ids so the payment schedule can also
// reject collisions against them.
func (d Deps) validateLines(ctx context.Context, v *apperr.Validation, lines []model.Line, ids map[string]bool, out *[]model.Line, currency *string) *big.Int {
	total := new(big.Int)
	for i, l := range lines {
		field := "terms.lines." + strconv.Itoa(i)
		if l.LineID == "" {
			l.LineID = uuid.NewString()
		}
		if uuid.Validate(l.LineID) != nil || ids[l.LineID] {
			v.Add(field+".lineId", "must be a unique ID")
		}
		ids[l.LineID] = true
		if l.Quantity < 1 || l.Quantity > 10_000 {
			v.Add(field+".quantity", "must be 1-10000")
		}
		price, ok := l.UnitPrice.Parse()
		if !ok || price.Sign() == 0 {
			v.Add(field+".unitPrice", "must be a positive amount in minor units with a currency code")
		} else {
			sameCurrency(v, currency, field+".unitPrice", l.UnitPrice.Currency)
			total.Add(total, new(big.Int).Mul(price, big.NewInt(int64(l.Quantity))))
		}
		if uuid.Validate(l.ModelID) != nil {
			v.Add(field+".modelId", "must be a valid ID")
		} else if _, err := d.catalog.Model(ctx, l.ModelID); err != nil {
			v.Add(field+".modelId", "unknown vehicle model")
		}
		*out = append(*out, l)
	}
	return total
}

// validateSchedule normalizes the payment schedule and returns the total
// amount scheduled. ids also tracks line IDs so schedule IDs cannot collide
// with them.
func validateSchedule(v *apperr.Validation, schedule []model.Installment, ids map[string]bool, out *[]model.Installment, currency *string) *big.Int {
	scheduled := new(big.Int)
	for i, p := range schedule {
		field := "terms.paymentSchedule." + strconv.Itoa(i)
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		if uuid.Validate(p.ID) != nil || ids[p.ID] {
			v.Add(field+".id", "must be a unique ID")
		}
		ids[p.ID] = true
		amount, ok := p.Amount.Parse()
		if !ok || amount.Sign() == 0 {
			v.Add(field+".amount", "must be a positive amount in minor units with a currency code")
		} else {
			sameCurrency(v, currency, field+".amount", p.Amount.Currency)
			scheduled.Add(scheduled, amount)
		}
		if _, err := time.Parse(time.DateOnly, p.DueDate); err != nil {
			v.Add(field+".dueDate", "must be a date (YYYY-MM-DD)")
		}
		*out = append(*out, p)
	}
	return scheduled
}

func validateAudience(v *apperr.Validation, own string, in model.Audience) model.Audience {
	out := model.Audience{Mode: in.Mode, PartnerCompanyIDs: validate.UniqueIDs(v, "audience.partnerCompanyIds", in.PartnerCompanyIDs)}
	switch in.Mode {
	case "all-active":
		if len(out.PartnerCompanyIDs) > 0 {
			v.Add("audience.partnerCompanyIds", "must be empty for all-active")
		}
	case "selected":
		if len(out.PartnerCompanyIDs) == 0 || len(out.PartnerCompanyIDs) > 500 {
			v.Add("audience.partnerCompanyIds", "select 1-500 partners")
		}
		if slices.Contains(out.PartnerCompanyIDs, own) {
			v.Add("audience.partnerCompanyIds", "cannot include your own company")
		}
	default:
		v.Add("audience.mode", "must be all-active or selected")
	}
	return out
}
