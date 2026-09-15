package money

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func testCurrency(t *testing.T, version string) Currency {
	t.Helper()
	currency, err := NewCurrency("USD", 2, version)
	if err != nil {
		t.Fatalf("NewCurrency: %v", err)
	}
	return currency
}

func TestNewAcceptsCanonicalAmountBoundaries(t *testing.T) {
	t.Parallel()
	currency := testCurrency(t, "2026-01")
	for _, value := range []string{"0", "1", "-1", strings.Repeat("9", 38), "-" + strings.Repeat("9", 38)} {
		value := value
		t.Run(value, func(t *testing.T) {
			got, err := New(value, currency)
			if err != nil {
				t.Fatalf("New(%q): %v", value, err)
			}
			if got.AmountMinor() != value {
				t.Fatalf("amount = %q, want %q", got.AmountMinor(), value)
			}
		})
	}
}

func TestNewRejectsNonCanonicalAndOverflowAmounts(t *testing.T) {
	t.Parallel()
	currency := testCurrency(t, "2026-01")
	tests := []struct {
		value string
		want  error
	}{
		{"", ErrInvalidAmount}, {"+1", ErrInvalidAmount}, {"-0", ErrInvalidAmount},
		{"01", ErrInvalidAmount}, {"-01", ErrInvalidAmount}, {" 1", ErrInvalidAmount},
		{"1.0", ErrInvalidAmount}, {"1e3", ErrInvalidAmount}, {strings.Repeat("9", 39), ErrAmountOverflow},
	}
	for _, tt := range tests {
		if _, err := New(tt.value, currency); !errors.Is(err, tt.want) {
			t.Errorf("New(%q) error = %v, want %v", tt.value, err, tt.want)
		}
	}
}

func TestCurrencyDefinitionValidationAndVersionIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ code, version string }{{"usd", "v1"}, {"USD1", "v1"}, {"", "v1"}, {"USD", ""}, {"USD", " v1"}} {
		if _, err := NewCurrency(tc.code, 2, tc.version); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("NewCurrency(%q, %q) error = %v", tc.code, tc.version, err)
		}
	}

	v1 := testCurrency(t, "v1")
	v2 := testCurrency(t, "v2")
	left, _ := New("100", v1)
	right, _ := New("100", v2)
	if left.Equal(right) {
		t.Fatal("different exponent versions compared equal")
	}
	if _, err := left.Add(right); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add error = %v, want currency mismatch", err)
	}

	otherExponent, _ := NewCurrency("USD", 3, "v1")
	right, _ = New("100", otherExponent)
	if _, err := left.Sub(right); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Sub error = %v, want currency mismatch", err)
	}
}

func TestArithmeticIsExactAndChecksResultOverflow(t *testing.T) {
	t.Parallel()
	currency := testCurrency(t, "v1")
	a, _ := New("100", currency)
	b, _ := New("35", currency)
	sum, err := a.Add(b)
	if err != nil || sum.AmountMinor() != "135" {
		t.Fatalf("Add = %q, %v", sum.AmountMinor(), err)
	}
	difference, err := b.Sub(a)
	if err != nil || difference.AmountMinor() != "-65" {
		t.Fatalf("Sub = %q, %v", difference.AmountMinor(), err)
	}
	maximum, _ := New(strings.Repeat("9", 38), currency)
	one, _ := New("1", currency)
	if _, err := maximum.Add(one); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
	if a.AmountMinor() != "100" || b.AmountMinor() != "35" {
		t.Fatal("arithmetic mutated an operand")
	}
}

func TestJSONUsesCanonicalPublicShapeAndSelectedDefinition(t *testing.T) {
	t.Parallel()
	currency := testCurrency(t, "v7")
	value, _ := New("-1840000", currency)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"amountMinor":"-1840000","currency":"USD"}` {
		t.Fatalf("JSON = %s", encoded)
	}
	decoded, err := ParseJSON(encoded, currency)
	if err != nil || !decoded.Equal(value) {
		t.Fatalf("ParseJSON = %#v, %v", decoded, err)
	}

	wrong, _ := NewCurrency("EUR", 2, "v7")
	if _, err := ParseJSON(encoded, wrong); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("wrong currency error = %v", err)
	}
	for _, input := range []string{
		`{"amountMinor":1840000,"currency":"USD"}`,
		`{"amountMinor":"01","currency":"USD"}`,
		`{"amountMinor":"1","currency":"USD","rate":1}`,
		`{"amountMinor":"1","amountMinor":"2","currency":"USD"}`,
		`{"amountMinor":"1"}`,
		`{"amountMinor":"1","currency":"USD"} {}`,
	} {
		if _, err := ParseJSON([]byte(input), currency); err == nil {
			t.Errorf("ParseJSON(%s) unexpectedly succeeded", input)
		}
	}
}

func TestMoneyConcurrentReadsAndArithmeticDoNotMutate(t *testing.T) {
	currency := testCurrency(t, "v1")
	base, _ := New("10000000000000000000000000000000000000", currency)
	delta, _ := New("1", currency)
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				if _, err := base.Add(delta); err != nil {
					t.Errorf("Add: %v", err)
				}
				_ = base.AmountMinor()
			}
		}()
	}
	wait.Wait()
	if base.AmountMinor() != "10000000000000000000000000000000000000" {
		t.Fatal("base value was mutated")
	}
}
