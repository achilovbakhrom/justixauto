package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP baseline). They are encoded into every hash, so
// raising them later still verifies old hashes.
const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 3
	argonThreads   = 2
	argonKeyLen    = 32
	argonSaltLen   = 16
)

var errBadHash = errors.New("identity: malformed password hash")

// hashPassword returns a PHC-formatted Argon2id hash.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// verifyPassword checks a password against a PHC-formatted Argon2id hash in constant time.
func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want))) //nolint:gosec // want is base64-decoded from a stored hash produced by hashPassword; its length is bounded by the fixed hash size, never user-controlled
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is verified for unknown logins so response time does not reveal
// whether an account exists.
var dummyHash, _ = hashPassword("justix-dummy-password")

func validatePassword(v interface{ Add(string, string) }, field, password, confirmation string) {
	switch n := len([]rune(password)); {
	case n < 12:
		v.Add(field, "at least 12 characters")
	case n > 128:
		v.Add(field, "at most 128 characters")
	case password != confirmation:
		v.Add(field+"Confirmation", "does not match")
	}
}

// validLogin trims a sign-in login: 3-100 characters without spaces.
func validLogin(v interface{ Add(string, string) }, field, login string) string {
	login = strings.TrimSpace(login)
	if n := len([]rune(login)); n < 3 || n > 100 || strings.ContainsAny(login, " \t\n") {
		v.Add(field, "3-100 characters without spaces")
	}
	return login
}
