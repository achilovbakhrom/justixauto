package identity

import (
	"slices"
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected hash format %q", hash)
	}
	if ok, err := verifyPassword("correct horse battery staple", hash); err != nil || !ok {
		t.Fatalf("correct password rejected: %v", err)
	}
	if ok, _ := verifyPassword("wrong password", hash); ok {
		t.Fatal("wrong password accepted")
	}
	other, _ := hashPassword("correct horse battery staple")
	if other == hash {
		t.Fatal("hashes must be salted")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$a$b", "$argon2id$v=19$m=x$a$b"} {
		if _, err := verifyPassword("x", bad); err == nil {
			t.Fatalf("malformed hash %q accepted", bad)
		}
	}
}

func TestSystemRolePermissionsAreCatalogued(t *testing.T) {
	for role, perms := range systemRolePermissions {
		for _, p := range perms {
			if _, ok := permissionInfo(p); !ok {
				t.Fatalf("%s grants uncatalogued permission %s", role, p)
			}
		}
	}
	// Administration powers never include company business permissions.
	if slices.Contains(systemRolePermissions[RolePlatformAdmin], PermCompanyEdit) {
		t.Fatal("platform admin must not implicitly edit companies as a member")
	}
}
