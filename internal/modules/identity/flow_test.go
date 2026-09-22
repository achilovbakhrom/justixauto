package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/idempotency"
)

// These tests exercise the real HTTP API against PostgreSQL. TEST_DATABASE_URL
// must point to a disposable database: identity tables are emptied.

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	m, err := database.NewMigrator(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	m.Close()
	db, err := database.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`TRUNCATE identity.audit_events, identity.session_branches, identity.sessions,
		identity.membership_branches, identity.memberships, identity.branches, identity.user_roles,
		identity.role_permissions, identity.users, identity.companies CASCADE;
		DELETE FROM identity.roles WHERE system_key IS NULL`).Error
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

type env struct {
	t     *testing.T
	srv   *httptest.Server
	clock *clock
	mod   *Module
}

func newEnv(t *testing.T) *env {
	db := openTestDB(t)
	if err := db.Exec("TRUNCATE platform.idempotency_keys").Error; err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	mod, err := New(db, Config{Cookie: CookieConfig{Secure: false}, Session: DefaultSessionConfig, MFAKey: key, Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	e := httpx.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	mod.Register(e.Group("/api/v1", mod.Authenticate(), idempotency.Middleware(db, clk.now, "/identity/session/")))
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, clock: clk, mod: mod}
}

// client is one browser: its own cookie jar and CSRF token.
type client struct {
	env  *env
	http *http.Client
	csrf string
	totp []byte
}

func (e *env) browser() *client {
	jar, _ := cookiejar.New(nil)
	return &client{env: e, http: &http.Client{Jar: jar}}
}

type response struct {
	status int
	header http.Header
	body   map[string]any
}

func (r response) data() map[string]any { d, _ := r.body["data"].(map[string]any); return d }
func (r response) items() []any         { i, _ := r.body["items"].([]any); return i }
func (r response) code() string {
	e, _ := r.body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (c *client) do(method, path string, body any, headers ...string) response {
	c.env.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.env.srv.URL+"/api/v1/identity"+path, reader)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if method == http.MethodPost && !strings.HasPrefix(path, "/session/") {
		req.Header.Set("Idempotency-Key", uuid.NewString()) // tests may override
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.env.t.Fatal(err)
	}
	defer res.Body.Close()
	out := response{status: res.StatusCode, header: res.Header, body: map[string]any{}}
	_ = json.NewDecoder(res.Body).Decode(&out.body)
	if token := res.Header.Get("X-CSRF-Token"); token != "" {
		c.csrf = token
	}
	return out
}

func (c *client) login(login, password string) response {
	return c.do(http.MethodPost, "/session/login", map[string]string{"login": login, "password": password})
}

// signIn logs in and, for users with MFA, answers the challenge.
func (c *client) signIn(login, password string) response {
	r := c.login(login, password)
	if id, ok := r.data()["challengeId"].(string); ok {
		r = c.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": c.code()})
	}
	return r
}

func expect(t *testing.T, r response, status int, code ...string) {
	t.Helper()
	if r.status != status || (len(code) > 0 && r.code() != code[0]) {
		t.Fatalf("want %d %v, got %d %v", status, code, r.status, r.body)
	}
}

func ifMatch(rev any) []string { return []string{"If-Match", `"` + rev.(string) + `"`} }

const adminPassword = "admin-password-123"

func (e *env) bootstrap() *client {
	e.t.Helper()
	_, err := e.mod.Users.Bootstrap(context.Background(), BootstrapInput{
		DisplayName: "Platform Admin", Login: "admin", Email: "admin@justix.test", Password: adminPassword})
	if err != nil {
		e.t.Fatal(err)
	}
	admin := e.browser()
	expect(e.t, admin.login("admin", adminPassword), http.StatusOK)
	admin.enrollMFA()
	return admin
}

// enrollMFA sets up TOTP for the signed-in user and remembers the secret.
func (c *client) enrollMFA() []string {
	t := c.env.t
	t.Helper()
	start := c.do(http.MethodPost, "/session/mfa/enrollment", nil)
	expect(t, start, http.StatusCreated)
	secret, err := b32.DecodeString(start.data()["secret"].(string))
	if err != nil {
		t.Fatal(err)
	}
	c.totp = secret
	confirm := c.do(http.MethodPost, "/session/mfa/enrollment/"+start.data()["enrollmentId"].(string)+"/confirm",
		map[string]string{"code": c.code()})
	expect(t, confirm, http.StatusOK)
	var codes []string
	for _, rc := range confirm.data()["recoveryCodes"].([]any) {
		codes = append(codes, rc.(string))
	}
	return codes
}

// code returns a fresh TOTP code, moving the clock to the next 30s step so
// the code has not been used yet.
func (c *client) code() string {
	c.env.clock.add(totpStep * time.Second)
	return totpCode(c.totp, c.env.clock.now())
}

func company(name, registration string) map[string]any {
	return map[string]any{"name": name, "country": map[string]string{"key": "UZ", "label": "Uzbekistan"},
		"registration": registration, "email": "office@" + registration + ".test"}
}

func provider(kind, name, registration, login string) map[string]any {
	return map[string]any{"kind": kind, "company": company(name, registration), "firstAdmin": map[string]string{
		"displayName": name + " admin", "login": login, "email": login + "@provider.test",
		"password": "provider-password-1", "passwordConfirmation": "provider-password-1"}}
}

func TestBootstrapIsSingleUse(t *testing.T) {
	e := newEnv(t)
	e.bootstrap()
	_, err := e.mod.Users.Bootstrap(context.Background(), BootstrapInput{
		DisplayName: "Second", Login: "admin2", Email: "admin2@justix.test", Password: adminPassword})
	var coded *apperr.Error
	if !errors.As(err, &coded) || coded.Code != "already_bootstrapped" {
		t.Fatalf("second bootstrap: %v", err)
	}
}

func TestLoginSessionAndCSRF(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()
	anon := e.browser()

	expect(t, anon.do(http.MethodGet, "/session", nil), http.StatusUnauthorized, "unauthenticated")
	expect(t, anon.login("admin", "wrong-password-1"), http.StatusUnauthorized, "invalid_credentials")
	expect(t, anon.login("nobody", "wrong-password-1"), http.StatusUnauthorized, "invalid_credentials")

	s := admin.do(http.MethodGet, "/session", nil)
	expect(t, s, http.StatusOK)
	if s.header.Get("Cache-Control") != "no-store" || admin.csrf == "" || s.body["revision"] != "1" {
		t.Fatalf("session metadata: %v %v", s.header, s.body)
	}
	if s.data()["setup"].(map[string]any)["next"] != "company" {
		t.Fatalf("setup: %v", s.data()["setup"])
	}

	// State-changing requests need the CSRF token and an allowed Origin.
	token := admin.csrf
	admin.csrf = ""
	expect(t, admin.do(http.MethodPost, "/companies", map[string]any{"company": company("X", "1")}), http.StatusForbidden, "csrf_invalid")
	admin.csrf = token
	expect(t, admin.do(http.MethodPost, "/session/logout", nil, "Origin", "https://evil.test"), http.StatusForbidden, "origin_denied")

	// Idle timeout ends the session.
	e.clock.add(31 * time.Minute)
	expect(t, admin.do(http.MethodGet, "/session", nil), http.StatusUnauthorized)

	// Logout revokes the session.
	expect(t, admin.signIn("admin", adminPassword), http.StatusOK)
	expect(t, admin.do(http.MethodPost, "/session/logout", nil), http.StatusNoContent)
	expect(t, admin.do(http.MethodGet, "/session", nil), http.StatusUnauthorized)
}

func TestLoginLockout(t *testing.T) {
	e := newEnv(t)
	e.bootstrap()
	c := e.browser()
	for i := 0; i < DefaultSessionConfig.LockoutThreshold; i++ {
		expect(t, c.login("admin", "wrong-password-1"), http.StatusUnauthorized)
	}
	r := c.login("admin", adminPassword)
	expect(t, r, http.StatusTooManyRequests, "rate_limited")
	if r.header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	e.clock.add(DefaultSessionConfig.LockoutDuration + time.Second)
	expect(t, c.login("admin", adminPassword), http.StatusOK)
}

func TestProviderProvisioningAndCompanyContext(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()

	created := admin.do(http.MethodPost, "/admin/provider-companies", provider("bank", "Capital Bank", "BANK-1", "capital"))
	expect(t, created, http.StatusCreated)
	bank := created.data()["company"].(map[string]any)
	if bank["access"] != "draft" || bank["kind"] != "bank" {
		t.Fatalf("provider company: %v", bank)
	}

	// An existing login rejects everything: no second company is created.
	expect(t, admin.do(http.MethodPost, "/admin/provider-companies", provider("mfo", "Other MFO", "MFO-1", "capital")), http.StatusConflict, "user_exists")
	expect(t, admin.do(http.MethodPost, "/admin/provider-companies", provider("mfo", "Dup", "bank-1", "dup")), http.StatusConflict, "company_duplicate")
	if n := len(admin.do(http.MethodGet, "/admin/companies", nil).items()); n != 1 {
		t.Fatalf("companies after rejected provisioning: %d", n)
	}
	expect(t, admin.do(http.MethodPost, "/admin/provider-companies", provider("seller", "S", "S-1", "s")), http.StatusUnprocessableEntity)

	// Activation needs a reason and the current revision.
	path := "/admin/companies/" + bank["id"].(string)
	expect(t, admin.do(http.MethodPost, path+"/activate", map[string]string{"reason": "contract signed"}), http.StatusPreconditionRequired)
	expect(t, admin.do(http.MethodPost, path+"/activate", map[string]string{"reason": ""}, ifMatch("1")...), http.StatusUnprocessableEntity)
	expect(t, admin.do(http.MethodPost, path+"/suspend", map[string]string{"reason": "x"}, ifMatch("1")...), http.StatusConflict, "invalid_transition")
	expect(t, admin.do(http.MethodPost, path+"/activate", map[string]string{"reason": "contract signed"}, ifMatch("1")...), http.StatusOK)
	expect(t, admin.do(http.MethodPost, path+"/suspend", map[string]string{"reason": "x"}, ifMatch("1")...), http.StatusPreconditionFailed, "stale_revision")

	// The provider admin signs in and works inside its company.
	banker := e.browser()
	s := banker.login("capital", "provider-password-1")
	expect(t, s, http.StatusOK)
	companies := s.data()["accessibleCompanies"].([]any)
	if len(companies) != 1 || companies[0].(map[string]any)["access"] != "active" {
		t.Fatalf("accessible companies: %v", companies)
	}
	expect(t, banker.do(http.MethodGet, "/admin/users", nil), http.StatusForbidden, "permission_denied")

	bankID := bank["id"].(string)
	ctx := banker.do(http.MethodPut, "/session/context", map[string]any{"companyId": bankID}, ifMatch("1")...)
	expect(t, ctx, http.StatusOK)
	if ctx.body["revision"] != "2" {
		t.Fatalf("context: %v", ctx.body)
	}
	expect(t, banker.do(http.MethodPut, "/session/context", map[string]any{"companyId": bankID}, ifMatch("1")...), http.StatusPreconditionFailed)
	if next := banker.do(http.MethodGet, "/session", nil).data()["setup"].(map[string]any)["next"]; next != "branch" {
		t.Fatalf("setup after company: %v", next)
	}

	branch := banker.do(http.MethodPost, "/companies/"+bankID+"/branches", map[string]any{"name": "Head office", "warehouse": map[string]string{"mode": "none"}})
	expect(t, branch, http.StatusCreated)
	expect(t, banker.do(http.MethodPost, "/companies/"+bankID+"/branches", map[string]any{"name": "head office"}), http.StatusConflict, "branch_duplicate")
	branchID := branch.data()["id"].(string)
	scope := banker.do(http.MethodPut, "/session/branch-scope", map[string]any{"mode": "SELECTED", "branchIds": []string{branchID}}, ifMatch("2")...)
	expect(t, scope, http.StatusOK)
	expect(t, banker.do(http.MethodPut, "/session/branch-scope", map[string]any{"mode": "SELECTED", "branchIds": []string{bankID}}, ifMatch("3")...), http.StatusUnprocessableEntity)

	// Company editing: members with company.edit; others do not see the company.
	edit := company("Capital Bank JSC", "BANK-1")
	expect(t, banker.do(http.MethodPatch, "/companies/"+bankID, edit, ifMatch("2")...), http.StatusOK)
	seller := admin.do(http.MethodPost, "/companies", map[string]any{"company": company("Admin Motors", "SELL-1")})
	expect(t, seller, http.StatusForbidden) // platform admins do not hold company.create
	outsider := banker.do(http.MethodGet, "/companies/00000000-0000-4000-8000-00000000abcd", nil)
	expect(t, outsider, http.StatusNotFound)

	// Revoking the membership drops the working context immediately.
	userID := created.data()["admin"].(map[string]any)["id"].(string)
	ms := admin.do(http.MethodGet, "/admin/users/"+userID+"/memberships", nil).items()
	m := ms[0].(map[string]any)
	expect(t, admin.do(http.MethodPost, "/admin/memberships/"+m["id"].(string)+"/revoke", map[string]string{"reason": "left"}, ifMatch(m["revision"])...), http.StatusOK)
	after := banker.do(http.MethodGet, "/session", nil)
	if after.data()["context"].(map[string]any)["companyId"] != nil || len(after.data()["accessibleCompanies"].([]any)) != 0 {
		t.Fatalf("context after revocation: %v", after.data())
	}

	audit := admin.do(http.MethodGet, "/admin/audit?resourceType=company&resourceId="+bankID, nil)
	expect(t, audit, http.StatusOK)
	// Newest first; the rejected (stale) suspension left no record.
	var actions []string
	for _, item := range audit.items() {
		actions = append(actions, item.(map[string]any)["action"].(string))
	}
	want := []string{"company.updated", "company.activate", "company.provider_provisioned"}
	if !slices.Equal(actions, want) {
		t.Fatalf("audit actions: %v", actions)
	}
}

func TestUsersRolesAndGuards(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()
	me := admin.do(http.MethodGet, "/session", nil).data()["user"].(map[string]any)["id"].(string)

	// Custom roles: only known, assignable permissions.
	expect(t, admin.do(http.MethodPost, "/admin/roles", map[string]any{"name": "Seller", "permissionKeys": []string{"nope"}}), http.StatusUnprocessableEntity)
	expect(t, admin.do(http.MethodPost, "/admin/roles", map[string]any{"name": "Seller", "permissionKeys": []string{PermPlatformUsersManage}}), http.StatusUnprocessableEntity)
	role := admin.do(http.MethodPost, "/admin/roles", map[string]any{"name": "Seller manager", "permissionKeys": []string{PermCompanyCreate, PermCompanyEdit, PermBranchesCreate}})
	expect(t, role, http.StatusCreated)
	expect(t, admin.do(http.MethodPatch, "/admin/roles/"+PlatformAdminRoleID, map[string]any{"name": "x", "permissionKeys": []string{}}, ifMatch("1")...), http.StatusConflict, "system_role")

	// New users are pending and cannot sign in.
	u := admin.do(http.MethodPost, "/admin/users", map[string]any{"displayName": "Dilnoza", "email": "dilnoza@seller.test", "roleIds": []string{role.data()["id"].(string)}})
	expect(t, u, http.StatusCreated)
	if u.data()["status"] != "pending" || len(u.data()["roles"].([]any)) != 1 {
		t.Fatalf("created user: %v", u.data())
	}
	expect(t, admin.do(http.MethodPost, "/admin/users", map[string]any{"displayName": "Dup", "email": "DILNOZA@seller.test"}), http.StatusConflict, "user_exists")
	expect(t, admin.do(http.MethodPost, "/admin/users", map[string]any{"displayName": "X", "email": "x@y.test", "roleIds": []string{"00000000-0000-4000-8000-00000000ffff"}}), http.StatusUnprocessableEntity)

	// Self-protection: no self-suspension, no removing own platform admin role.
	expect(t, admin.do(http.MethodPost, "/admin/users/"+me+"/suspend", map[string]string{"reason": "oops"}, ifMatch("1")...), http.StatusConflict, "self_lockout")
	expect(t, admin.do(http.MethodPatch, "/admin/users/"+me, map[string]any{"displayName": "Me", "roleIds": []string{}}, ifMatch("1")...), http.StatusConflict, "self_lockout")

	// Suspending a provider admin ends their sessions at once.
	created := admin.do(http.MethodPost, "/admin/provider-companies", provider("insurance", "Safe Insurance", "INS-1", "safe"))
	expect(t, created, http.StatusCreated)
	insurer := e.browser()
	expect(t, insurer.login("safe", "provider-password-1"), http.StatusOK)
	id := created.data()["admin"].(map[string]any)["id"].(string)
	expect(t, admin.do(http.MethodPost, "/admin/users/"+id+"/suspend", map[string]string{"reason": "audit"}, ifMatch("1")...), http.StatusOK)
	expect(t, insurer.do(http.MethodGet, "/session", nil), http.StatusUnauthorized)
	expect(t, insurer.login("safe", "provider-password-1"), http.StatusForbidden, "account_suspended")
	restored := admin.do(http.MethodPost, "/admin/users/"+id+"/restore", map[string]string{"reason": "cleared"}, ifMatch("2")...)
	expect(t, restored, http.StatusOK)
	if restored.data()["status"] != "active" {
		t.Fatalf("restored: %v", restored.data())
	}
	expect(t, insurer.login("safe", "provider-password-1"), http.StatusOK)
}

func TestMFA(t *testing.T) {
	e := newEnv(t)
	_, err := e.mod.Users.Bootstrap(context.Background(), BootstrapInput{
		DisplayName: "Platform Admin", Login: "admin", Email: "admin@justix.test", Password: adminPassword})
	if err != nil {
		t.Fatal(err)
	}
	admin := e.browser()
	expect(t, admin.login("admin", adminPassword), http.StatusOK)

	// Sensitive permissions need MFA; reading the directory does not.
	expect(t, admin.do(http.MethodGet, "/admin/users", nil), http.StatusForbidden, "mfa_enrollment_required")
	expect(t, admin.do(http.MethodGet, "/admin/companies", nil), http.StatusOK)

	recovery := admin.enrollMFA()
	if len(recovery) != recoveryCodeCount {
		t.Fatalf("recovery codes: %v", recovery)
	}
	s := admin.do(http.MethodGet, "/session", nil)
	if s.data()["mfa"].(map[string]any)["enrolled"] != true {
		t.Fatalf("mfa view: %v", s.data()["mfa"])
	}
	expect(t, admin.do(http.MethodGet, "/admin/users", nil), http.StatusOK)
	expect(t, admin.do(http.MethodPost, "/session/mfa/enrollment", nil), http.StatusConflict, "mfa_already_enrolled")

	// The second factor goes stale; step-up renews it. A code works only once.
	e.clock.add(mfaFreshness + time.Second)
	expect(t, admin.do(http.MethodGet, "/admin/users", nil), http.StatusForbidden, "mfa_required")
	code := admin.code()
	expect(t, admin.do(http.MethodPost, "/session/mfa/step-up", map[string]string{"code": code}), http.StatusOK)
	expect(t, admin.do(http.MethodPost, "/session/mfa/step-up", map[string]string{"code": code}), http.StatusUnauthorized, "invalid_mfa_code")
	expect(t, admin.do(http.MethodGet, "/admin/users", nil), http.StatusOK)

	// Login now requires the second factor; the challenge is bound to the browser.
	expect(t, admin.do(http.MethodPost, "/session/logout", nil), http.StatusNoContent)
	ch := admin.login("admin", adminPassword)
	expect(t, ch, http.StatusOK)
	id, _ := ch.data()["challengeId"].(string)
	if id == "" || ch.data()["required"] != "mfa" {
		t.Fatalf("challenge: %v", ch.body)
	}
	expect(t, admin.do(http.MethodGet, "/session", nil), http.StatusUnauthorized)
	thief := e.browser() // same challenge ID without the challenge cookie
	thief.totp = admin.totp
	expect(t, thief.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": thief.code()}), http.StatusUnauthorized, "invalid_mfa_code")
	expect(t, admin.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": "000000"}), http.StatusUnauthorized, "invalid_mfa_code")
	expect(t, admin.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": admin.code()}), http.StatusOK)
	expect(t, admin.do(http.MethodGet, "/admin/users", nil), http.StatusOK) // login with MFA is fresh
	expect(t, admin.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": admin.code()}), http.StatusUnauthorized)

	// A recovery code replaces the authenticator once.
	expect(t, admin.do(http.MethodPost, "/session/logout", nil), http.StatusNoContent)
	id = admin.login("admin", adminPassword).data()["challengeId"].(string)
	expect(t, admin.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": strings.ToUpper(recovery[0])}), http.StatusOK)
	expect(t, admin.do(http.MethodPost, "/session/mfa/step-up", map[string]string{"code": recovery[0]}), http.StatusUnauthorized, "invalid_mfa_code")
}

func TestMFACodeGuessingLocksAccount(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()
	expect(t, admin.do(http.MethodPost, "/session/logout", nil), http.StatusNoContent)
	// Each login gives a new challenge, but wrong codes still count toward lockout.
	for i := 0; i < DefaultSessionConfig.LockoutThreshold; i++ {
		id := admin.login("admin", adminPassword).data()["challengeId"].(string)
		expect(t, admin.do(http.MethodPost, "/session/mfa/verify", map[string]string{"challengeId": id, "code": "000000"}), http.StatusUnauthorized)
	}
	expect(t, admin.login("admin", adminPassword), http.StatusTooManyRequests, "rate_limited")
}

func TestIdempotencyKey(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()
	key := uuid.NewString()
	body := map[string]any{"name": "Support", "permissionKeys": []string{PermPlatformDirectoryRead}}

	first := admin.do(http.MethodPost, "/admin/roles", body, "Idempotency-Key", key)
	expect(t, first, http.StatusCreated)
	retry := admin.do(http.MethodPost, "/admin/roles", body, "Idempotency-Key", key)
	expect(t, retry, http.StatusCreated)
	if retry.data()["id"] != first.data()["id"] || retry.header.Get("Idempotent-Replayed") != "true" || retry.header.Get("ETag") != `"1"` {
		t.Fatalf("replay: %v %v", retry.header, retry.body)
	}
	if n := len(admin.do(http.MethodGet, "/admin/roles", nil).items()); n != 3 { // 2 system + 1
		t.Fatalf("roles after retry: %d", n)
	}
	body["name"] = "Other"
	expect(t, admin.do(http.MethodPost, "/admin/roles", body, "Idempotency-Key", key), http.StatusConflict, "idempotency_key_reused")
	expect(t, admin.do(http.MethodPost, "/admin/roles", body, "Idempotency-Key", ""), http.StatusPreconditionRequired, "idempotency_key_required")
	expect(t, admin.do(http.MethodPost, "/admin/roles", body, "Idempotency-Key", "abc"), http.StatusUnprocessableEntity)

	// A failed request releases its key so the corrected retry can use it.
	bad := uuid.NewString()
	expect(t, admin.do(http.MethodPost, "/admin/roles", map[string]any{"name": ""}, "Idempotency-Key", bad), http.StatusUnprocessableEntity)
	expect(t, admin.do(http.MethodPost, "/admin/roles", map[string]any{"name": ""}, "Idempotency-Key", bad), http.StatusUnprocessableEntity)
}

func TestSellerOnboardingAndAdminSetPasswords(t *testing.T) {
	e := newEnv(t)
	admin := e.bootstrap()

	seller := admin.do(http.MethodPost, "/admin/seller-companies", map[string]any{"company": company("Justix Motors", "SELL-1"),
		"firstAdmin": map[string]string{"displayName": "Owner", "login": "owner", "email": "owner@motors.test",
			"password": "owner-password-12", "passwordConfirmation": "owner-password-12"}})
	expect(t, seller, http.StatusCreated)
	if seller.data()["company"].(map[string]any)["kind"] != "seller" {
		t.Fatalf("seller company: %v", seller.data())
	}
	owner := e.browser()
	expect(t, owner.login("owner", "owner-password-12"), http.StatusOK)

	// A pending staff member gets a login and an initial password from the admin.
	u := admin.do(http.MethodPost, "/admin/users", map[string]any{"displayName": "Staff", "email": "staff@motors.test"})
	expect(t, u, http.StatusCreated)
	id := u.data()["id"].(string)
	expect(t, admin.do(http.MethodPost, "/admin/users/"+id+"/password", map[string]string{"password": "initial-pass-123", "passwordConfirmation": "initial-pass-123"}, ifMatch("1")...), http.StatusUnprocessableEntity)
	expect(t, admin.do(http.MethodPost, "/admin/users/"+id+"/password", map[string]string{"login": "OWNER", "password": "initial-pass-123", "passwordConfirmation": "initial-pass-123"}, ifMatch("1")...), http.StatusConflict, "login_taken")
	set := admin.do(http.MethodPost, "/admin/users/"+id+"/password", map[string]string{"login": "staff", "password": "initial-pass-123", "passwordConfirmation": "initial-pass-123"}, ifMatch("1")...)
	expect(t, set, http.StatusOK)
	if set.data()["status"] != "active" {
		t.Fatalf("after password set: %v", set.data())
	}
	me := admin.do(http.MethodGet, "/session", nil).data()["user"].(map[string]any)["id"].(string)
	expect(t, admin.do(http.MethodPost, "/admin/users/"+me+"/password", map[string]string{"password": "x", "passwordConfirmation": "x"}, ifMatch("1")...), http.StatusConflict, "use_own_password_change")

	// The staff member must choose their own password before anything else.
	staff := e.browser()
	s := staff.login("staff", "initial-pass-123")
	expect(t, s, http.StatusOK)
	if s.data()["user"].(map[string]any)["passwordChangeRequired"] != true {
		t.Fatalf("session: %v", s.data()["user"])
	}
	expect(t, staff.do(http.MethodPut, "/session/context", map[string]any{"companyId": nil}, ifMatch("1")...), http.StatusForbidden, "password_change_required")
	expect(t, staff.do(http.MethodPost, "/session/password", map[string]string{"currentPassword": "wrong-pass-1234", "newPassword": "my-own-password-1", "newPasswordConfirmation": "my-own-password-1"}), http.StatusUnprocessableEntity)
	expect(t, staff.do(http.MethodPost, "/session/password", map[string]string{"currentPassword": "initial-pass-123", "newPassword": "initial-pass-123", "newPasswordConfirmation": "initial-pass-123"}), http.StatusUnprocessableEntity)
	changed := staff.do(http.MethodPost, "/session/password", map[string]string{"currentPassword": "initial-pass-123", "newPassword": "my-own-password-1", "newPasswordConfirmation": "my-own-password-1"})
	expect(t, changed, http.StatusOK)
	if changed.data()["user"].(map[string]any)["passwordChangeRequired"] != false {
		t.Fatalf("after change: %v", changed.data()["user"])
	}
	expect(t, staff.do(http.MethodPut, "/session/context", map[string]any{"companyId": nil}, ifMatch("1")...), http.StatusOK)
	expect(t, e.browser().login("staff", "initial-pass-123"), http.StatusUnauthorized)
	expect(t, e.browser().login("staff", "my-own-password-1"), http.StatusOK)
}
