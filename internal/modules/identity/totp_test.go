package identity

import (
	"encoding/hex"
	"testing"
	"time"
)

// RFC 6238 appendix B test vectors (SHA-1, truncated to 6 digits).
func TestTOTPVectors(t *testing.T) {
	secret, _ := hex.DecodeString("3132333435363738393031323334353637383930")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpCode(secret, time.Unix(unix, 0)); got != want {
			t.Fatalf("t=%d: got %s want %s", unix, got, want)
		}
	}
}

func TestTOTPVerifyWindowAndReplay(t *testing.T) {
	secret, _ := newTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	code := totpCode(secret, now.Add(-30*time.Second))
	counter, ok := verifyTOTP(secret, code, now, 0)
	if !ok {
		t.Fatal("previous step must be accepted for clock drift")
	}
	if _, ok := verifyTOTP(secret, code, now, counter); ok {
		t.Fatal("a used step must not be accepted again")
	}
	if _, ok := verifyTOTP(secret, totpCode(secret, now.Add(-90*time.Second)), now, 0); ok {
		t.Fatal("codes older than the window must be rejected")
	}
}

func TestSecretBox(t *testing.T) {
	box, err := newSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := box.seal([]byte("secret"))
	if plain, err := box.open(sealed); err != nil || string(plain) != "secret" {
		t.Fatalf("round trip: %q %v", plain, err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := box.open(sealed); err == nil {
		t.Fatal("tampered secret accepted")
	}
	if _, err := newSecretBox([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}
