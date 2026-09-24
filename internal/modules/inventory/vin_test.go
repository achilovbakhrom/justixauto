package inventory

import "testing"

func TestNormalizeVIN(t *testing.T) {
	for raw, want := range map[string]string{
		" wvwzzz1jz3w386752 ": "WVWZZZ1JZ3W386752", //nolint:gocritic // intentional: raw input with surrounding whitespace to exercise trimming
		"1HGCM82633A004352":   "1HGCM82633A004352",
	} {
		if got, ok := normalizeVIN(raw); !ok || got != want {
			t.Fatalf("%q: got %q %v", raw, got, ok)
		}
	}
	for _, bad := range []string{"", "1HGCM82633A00435", "1HGCM82633A0043521", "1HGCM82633A00435I", "1HGCM82633A00435O", "1HGCM82633A00435Q", "1HGCM82633A00435-"} {
		if _, ok := normalizeVIN(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}
