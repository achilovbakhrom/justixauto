// Package testkit runs module tests end to end: a real PostgreSQL database
// (TEST_DATABASE_URL, emptied per test), the identity module for sign-in, and
// HTTP clients that behave like a browser (cookies, CSRF, Idempotency-Key).
// Use it from _test.go files only.
package testkit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/app"
	"justixauto/internal/modules/identity"
	"justixauto/internal/platform/database"
)

// Clock is a controllable time source shared by the server and the test.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *Clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *Clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// DB migrates TEST_DATABASE_URL and empties all application tables. The test
// is skipped when the variable is not set.
func DB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database (bash tools/test-go.sh)")
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
	var tables []string
	err = db.Raw(`SELECT quote_ident(schemaname) || '.' || quote_ident(tablename) FROM pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'public') AND tablename <> 'roles'`).Scan(&tables).Error
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("TRUNCATE " + strings.Join(tables, ", ") + " CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM identity.roles WHERE system_key IS NULL").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// Env is a running API with identity plus the modules under test.
type Env struct {
	T        *testing.T
	DB       *gorm.DB
	Clock    *Clock
	Identity *identity.Module
	srv      *httptest.Server
}

// New starts the full API (all modules, wired as in production) on a clean
// database with a controllable clock.
func New(t *testing.T) *Env {
	t.Helper()
	db := DB(t)
	clock := &Clock{t: time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	e, idm, err := app.New(db, app.Config{Session: identity.DefaultSessionConfig, MFAKey: key, Now: clock.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return &Env{T: t, DB: db, Clock: clock, Identity: idm, srv: srv}
}

// Client is one browser session.
type Client struct {
	env  *Env
	http *http.Client
	csrf string
	totp []byte
	// UserID and CompanyID are filled by the helpers that create them.
	UserID, CompanyID string
}

func (e *Env) Browser() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{env: e, http: &http.Client{Jar: jar}}
}

// Response is a decoded API response.
type Response struct {
	Status int
	Header http.Header
	Body   map[string]any
}

func (r Response) Data() map[string]any { d, _ := r.Body["data"].(map[string]any); return d }
func (r Response) Items() []any         { i, _ := r.Body["items"].([]any); return i }
func (r Response) Revision() string     { s, _ := r.Body["revision"].(string); return s }
func (r Response) Code() string {
	e, _ := r.Body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

// Do sends a request to /api/v1+path. headers are name/value pairs; POSTs get
// a fresh Idempotency-Key unless one is given.
func (c *Client) Do(method, path string, body any, headers ...string) Response {
	t := c.env.T
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.env.srv.URL+"/api/v1"+path, reader)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", uuid.NewString())
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := Response{Status: res.StatusCode, Header: res.Header, Body: map[string]any{}}
	_ = json.NewDecoder(res.Body).Decode(&out.Body)
	if token := res.Header.Get("X-CSRF-Token"); token != "" {
		c.csrf = token
	}
	return out
}

// IfMatch builds the If-Match header pair for Do.
func IfMatch(revision string) []string { return []string{"If-Match", `"` + revision + `"`} }

// Expect fails the test unless the status (and optional error code) match.
func Expect(t *testing.T, r Response, status int, code ...string) {
	t.Helper()
	if r.Status != status || (len(code) > 0 && r.Code() != code[0]) {
		t.Fatalf("want %d %v, got %d %v", status, code, r.Status, r.Body)
	}
}

// SignIn logs in and answers the MFA challenge when the client has TOTP.
func (c *Client) SignIn(login, password string) Response {
	r := c.Do(http.MethodPost, "/identity/session/login", map[string]string{"login": login, "password": password})
	if id, ok := r.Data()["challengeId"].(string); ok {
		r = c.Do(http.MethodPost, "/identity/session/mfa/verify", map[string]string{"challengeId": id, "code": c.code()})
	}
	return r
}

// EnrollMFA sets up TOTP for the signed-in user.
func (c *Client) EnrollMFA() {
	t := c.env.T
	t.Helper()
	start := c.Do(http.MethodPost, "/identity/session/mfa/enrollment", nil)
	Expect(t, start, http.StatusCreated)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(start.Data()["secret"].(string))
	if err != nil {
		t.Fatal(err)
	}
	c.totp = secret
	Expect(t, c.Do(http.MethodPost, "/identity/session/mfa/enrollment/"+start.Data()["enrollmentId"].(string)+"/confirm",
		map[string]string{"code": c.code()}), http.StatusOK)
}

// code returns an unused TOTP code by moving the clock to the next 30s step.
func (c *Client) code() string {
	c.env.Clock.Add(30 * time.Second)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(c.env.Clock.Now().Unix()/30))
	mac := hmac.New(sha1.New, c.totp)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	o := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[o:o+4])&0x7fffffff)%1_000_000)
}

const adminPassword = "admin-password-123"

// Admin bootstraps the platform administrator (once per Env) with MFA.
func (e *Env) Admin() *Client {
	t := e.T
	t.Helper()
	_, err := e.Identity.Users.Bootstrap(context.Background(), identity.BootstrapInput{
		DisplayName: "Platform Admin", Login: "admin", Email: "admin@justix.test", Password: adminPassword})
	if err != nil {
		t.Fatal(err)
	}
	admin := e.Browser()
	Expect(t, admin.SignIn("admin", adminPassword), http.StatusOK)
	admin.EnrollMFA()
	return admin
}

// CompanyUser creates an active seller company with a first administrator who additionally
// holds a custom role with perms, signs them in and selects the company.
func (e *Env) CompanyUser(admin *Client, name string, perms ...string) *Client {
	t := e.T
	t.Helper()
	login := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	created := admin.Do(http.MethodPost, "/identity/admin/seller-companies", map[string]any{
		"company": map[string]any{"name": name, "country": map[string]string{"key": "UZ", "label": "Uzbekistan"},
			"registration": "REG-" + login, "email": "office@" + login + ".test"},
		"firstAdmin": map[string]string{"displayName": name + " user", "login": login, "email": login + "@company.test",
			"password": "company-password-1", "passwordConfirmation": "company-password-1"}})
	Expect(t, created, http.StatusCreated)
	userID := created.Data()["admin"].(map[string]any)["id"].(string)
	companyID := created.Data()["company"].(map[string]any)["id"].(string)
	Expect(t, admin.Do(http.MethodPost, "/identity/admin/companies/"+companyID+"/activate",
		map[string]string{"reason": "onboarded"}, IfMatch("1")...), http.StatusOK)
	role := admin.Do(http.MethodPost, "/identity/admin/roles", map[string]any{"name": name + " staff", "permissionKeys": perms})
	Expect(t, role, http.StatusCreated)
	user := admin.Do(http.MethodGet, "/identity/admin/users/"+userID, nil)
	Expect(t, admin.Do(http.MethodPatch, "/identity/admin/users/"+userID, map[string]any{"displayName": name + " user",
		"roleIds": []string{identity.CompanyAdminRoleID, role.Data()["id"].(string)}}, IfMatch(user.Revision())...), http.StatusOK)

	c := e.Browser()
	s := c.SignIn(login, "company-password-1")
	Expect(t, s, http.StatusOK)
	Expect(t, c.Do(http.MethodPut, "/identity/session/context", map[string]any{"companyId": companyID}, IfMatch(s.Revision())...), http.StatusOK)
	c.UserID, c.CompanyID = userID, companyID
	return c
}
