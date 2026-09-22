package identity

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"justixauto/internal/platform/httpx"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	e := httpx.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	NewCompanyHandler(newTestService()).Routes(e.Group("/api/v1").Group("/identity"))
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res, decoded
}

const companyJSON = `{"kind":"insurer","name":"Safe Insurance","country":"Uzbekistan","registrationNumber":"987"}`

func TestCompanyHTTPFlow(t *testing.T) {
	srv := newTestServer(t)

	res, created := call(t, srv, http.MethodPost, "/api/v1/identity/companies", companyJSON, nil)
	if res.StatusCode != http.StatusCreated || res.Header.Get("ETag") != `"1"` || created["status"] != "draft" {
		t.Fatalf("create: %d %v %v", res.StatusCode, res.Header.Get("ETag"), created)
	}
	path := "/api/v1/identity/companies/" + created["id"].(string)

	if res, _ := call(t, srv, http.MethodGet, path, "", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("get: %d", res.StatusCode)
	}
	if res, _ := call(t, srv, http.MethodPut, path, companyJSON, nil); res.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("update without If-Match: %d", res.StatusCode)
	}
	res, updated := call(t, srv, http.MethodPut, path, `{"name":"Renamed","country":"Uzbekistan","registrationNumber":"987"}`, map[string]string{"If-Match": `"1"`})
	if res.StatusCode != http.StatusOK || res.Header.Get("ETag") != `"2"` || updated["name"] != "Renamed" {
		t.Fatalf("update: %d %v", res.StatusCode, updated)
	}
	if res, _ := call(t, srv, http.MethodPost, path+"/status", `{"status":"active","reason":"ok"}`, map[string]string{"If-Match": `"1"`}); res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("stale status change: %d", res.StatusCode)
	}
	res, activated := call(t, srv, http.MethodPost, path+"/status", `{"status":"active","reason":"contract signed"}`, map[string]string{"If-Match": `"2"`})
	if res.StatusCode != http.StatusOK || activated["status"] != "active" {
		t.Fatalf("activate: %d %v", res.StatusCode, activated)
	}
	res, list := call(t, srv, http.MethodGet, "/api/v1/identity/companies?status=active", "", nil)
	if res.StatusCode != http.StatusOK || len(list["items"].([]any)) != 1 {
		t.Fatalf("list: %d %v", res.StatusCode, list)
	}
}

func TestCompanyHTTPErrors(t *testing.T) {
	srv := newTestServer(t)
	res, body := call(t, srv, http.MethodPost, "/api/v1/identity/companies", `{"kind":"dealer"}`, nil)
	if res.StatusCode != http.StatusUnprocessableEntity || body["fields"] == nil {
		t.Fatalf("validation: %d %v", res.StatusCode, body)
	}
	if res, _ := call(t, srv, http.MethodPost, "/api/v1/identity/companies", `{bad json`, nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed json: %d", res.StatusCode)
	}
	call(t, srv, http.MethodPost, "/api/v1/identity/companies", companyJSON, nil)
	if res, _ := call(t, srv, http.MethodPost, "/api/v1/identity/companies", companyJSON, nil); res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate: %d", res.StatusCode)
	}
	if res, _ := call(t, srv, http.MethodGet, "/api/v1/identity/companies/unknown", "", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: %d", res.StatusCode)
	}
	if res, _ := call(t, srv, http.MethodGet, "/api/v1/identity/companies?limit=abc", "", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", res.StatusCode)
	}
}
