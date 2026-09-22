package money

import "testing"

func TestParse(t *testing.T) {
	for _, ok := range []Money{{"0", "USD"}, {"1999", "UZS"}, {"99999999999999999999999999999999999999", "USD"}} {
		if _, valid := ok.Parse(); !valid {
			t.Fatalf("%v rejected", ok)
		}
	}
	for _, bad := range []Money{{"", "USD"}, {"01", "USD"}, {"-5", "USD"}, {"1.5", "USD"}, {"1e3", "USD"},
		{"100", "usd"}, {"100", "US"}, {"999999999999999999999999999999999999999", "USD"}} {
		if _, valid := bad.Parse(); valid {
			t.Fatalf("%v accepted", bad)
		}
	}
}
