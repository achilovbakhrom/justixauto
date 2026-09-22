package identity

import "testing"

func TestMFADisabledRequiresNoSecondFactor(t *testing.T) {
	if n := len((&AuthService{mfaOff: true}).mfaRequired()); n != 0 {
		t.Fatalf("switched off: %d permissions still need MFA", n)
	}
	if len((&AuthService{}).mfaRequired()) == 0 {
		t.Fatal("by default sensitive permissions must need MFA")
	}
}
