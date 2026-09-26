package service

import (
	"testing"

	"justixauto/internal/pkg/apperr"
)

func TestRequireCredentials(t *testing.T) {
	for _, c := range []struct{ login, password string }{{"", ""}, {"ivan", ""}, {"", "long-enough-password"}} {
		var v apperr.Validation
		requireCredentials(&v, c.login, c.password)
		if v.Err() == nil {
			t.Errorf("login %q password %q: want a validation error", c.login, c.password)
		}
	}
	var v apperr.Validation
	requireCredentials(&v, "ivan", "long-enough-password")
	if v.Err() != nil {
		t.Errorf("both given: unexpected error %v", v.Err())
	}
}
