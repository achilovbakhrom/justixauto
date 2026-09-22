package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP per RFC 6238: SHA-1, 6 digits, 30-second steps — what authenticator
// apps expect by default.
const (
	totpStep   = 30
	totpDigits = 6
	totpSkew   = 1 // accept one step before/after for clock drift
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() ([]byte, error) {
	secret := make([]byte, 20)
	_, err := rand.Read(secret)
	return secret, err
}

func hotp(secret []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, code%1_000_000)
}

// totpCode returns the code for time t (used by tests and verification).
func totpCode(secret []byte, t time.Time) string {
	return hotp(secret, uint64(t.Unix()/totpStep))
}

// verifyTOTP returns the matched step counter. Steps at or below lastCounter
// are rejected so a code cannot be replayed.
func verifyTOTP(secret []byte, code string, now time.Time, lastCounter int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	current := now.Unix() / totpStep
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		counter := current + d
		if counter <= lastCounter {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(secret, uint64(counter))), []byte(code)) == 1 {
			return counter, true
		}
	}
	return 0, false
}

func otpauthURI(secret []byte, account string) string {
	label := url.PathEscape("JustixAuto:" + account)
	return "otpauth://totp/" + label + "?secret=" + b32.EncodeToString(secret) + "&issuer=JustixAuto&digits=6&period=30"
}

// secretBox encrypts MFA secrets at rest with AES-256-GCM.
type secretBox struct{ aead cipher.AEAD }

func newSecretBox(key []byte) (*secretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("identity: MFA key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretBox{aead: aead}, nil
}

func (b *secretBox) seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

func (b *secretBox) open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("identity: sealed secret too short")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// newRecoveryCodes returns human-typable single-use codes (xxxxx-xxxxx, 50 bits).
func newRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, n)
	for i := range codes {
		raw := make([]byte, 7)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		s := strings.ToLower(b32.EncodeToString(raw))[:10]
		codes[i] = s[:5] + "-" + s[5:]
	}
	return codes, nil
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
}
