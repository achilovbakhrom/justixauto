package envx

import "testing"

func TestOr(t *testing.T) {
	t.Setenv("ENVX_SET", "value")
	t.Setenv("ENVX_EMPTY", "")
	for key, want := range map[string]string{"ENVX_SET": "value", "ENVX_EMPTY": "fallback", "ENVX_UNSET": "fallback"} {
		if got := Or(key, "fallback"); got != want {
			t.Errorf("Or(%q) = %q, want %q", key, got, want)
		}
	}
}
