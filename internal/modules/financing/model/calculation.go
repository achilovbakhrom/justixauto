package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"justixauto/internal/pkg/money"
)

// CalculationInput is what the seller (or the provider for terms) enters.
type CalculationInput struct {
	DownPayment  money.Money `json:"downPayment"`
	TermMonths   int         `json:"termMonths"`
	FirstDueDate string      `json:"firstDueDate"` // YYYY-MM-DD
}

type ScheduleRow struct {
	Number  int         `json:"number"`
	DueDate string      `json:"dueDate"`
	Total   money.Money `json:"total"`
	Balance money.Money `json:"balance"`
}

// Calculation is the contract's CalculationSnapshot for a partner program.
type Calculation struct {
	Kind           string           `json:"kind"`
	PolicyID       string           `json:"policyId"`
	PolicyVersion  int              `json:"policyVersion"`
	ProgramID      string           `json:"programId"`
	ProgramVersion int              `json:"programVersion"`
	Price          money.Money      `json:"price"`
	Input          CalculationInput `json:"input"`
	Output         struct {
		Markup         money.Money   `json:"markup"`
		Total          money.Money   `json:"total"`
		FinancedAmount money.Money   `json:"financedAmount"`
		Schedule       []ScheduleRow `json:"schedule"`
	} `json:"output"`
	CalculatedAt time.Time `json:"calculatedAt"`
}

// Digest identifies the exact calculation (what the seller reviewed).
func (c *Calculation) Digest() string {
	raw, _ := json.Marshal(c)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
