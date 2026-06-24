package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciimoo/hister/config"

	"github.com/gorilla/sessions"
)

func newPublicTokenTestServer(t *testing.T) (*config.Config, http.Handler) {
	return newTokenTestServer(t, true)
}

func newTokenTestServer(t *testing.T, public bool) (*config.Config, http.Handler) {
	t.Helper()
	cfg := config.CreateDefaultConfig()
	cfg.App.Directory = t.TempDir()
	cfg.App.AccessToken = "secret"
	cfg.App.Public = public
	cfg.Server.Address = "127.0.0.1:4433"
	if err := cfg.UpdateBaseURL("http://127.0.0.1:4433"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveRules(); err != nil {
		t.Fatal(err)
	}
	sessionStore = sessions.NewCookieStore([]byte(strings.Repeat("x", 32)))
	return cfg, registerEndpoints(cfg)
}

func TestPublicModeConfigResponse(t *testing.T) {
	_, handler := newPublicTokenTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Public         bool `json:"public"`
		Authenticated  bool `json:"authenticated"`
		CanWrite       bool `json:"canWrite"`
		HistoryEnabled bool `json:"historyEnabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Public {
		t.Fatal("public = false, want true")
	}
	if body.Authenticated {
		t.Fatal("authenticated = true, want false")
	}
	if body.CanWrite {
		t.Fatal("canWrite = true, want false")
	}
	if body.HistoryEnabled {
		t.Fatal("historyEnabled = true, want false")
	}
}

func TestPublicModeAllowsDocumentedPublicRoutes(t *testing.T) {
	cfg, handler := newPublicTokenTestServer(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(filePath, []byte("public file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Indexer.Directories = []*config.Directory{{Path: dir}}

	tests := []struct {
		name   string
		method string
		target string
		body   string
		want   int
	}{
		{name: "api docs", method: http.MethodGet, target: "/api", want: http.StatusOK},
		{name: "search", method: http.MethodGet, target: "/search?format=json", want: http.StatusBadRequest},
		{name: "file", method: http.MethodGet, target: "/api/file?path=" + filePath, want: http.StatusOK},
		{name: "mcp tools list", method: http.MethodPost, target: "/mcp", body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("%s %s status = %d, want %d; body=%s", tt.method, tt.target, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestPublicModeProtectsWriteRoutes(t *testing.T) {
	_, handler := newPublicTokenTestServer(t)
	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "delete", method: http.MethodPost, target: "/api/delete", body: `{"query":"*"}`},
		{name: "add", method: http.MethodPost, target: "/api/add", body: `{"url":"https://example.com"}`},
		{name: "rules", method: http.MethodGet, target: "/api/rules"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.target, rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestPublicModeAllowsAuthenticatedProtectedRoutes(t *testing.T) {
	_, handler := newPublicTokenTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/add", nil)
	req.Header.Set("Origin", "hister://")
	req.Header.Set("X-Access-Token", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/add status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPublicModeDisablesHistoryForAuthenticatedCallers(t *testing.T) {
	_, handler := newPublicTokenTestServer(t)
	anonymousReq := httptest.NewRequest(http.MethodPost, "/api/history", strings.NewReader(`{"query":"q","url":"https://example.com"}`))
	anonymousReq.Header.Set("Origin", "hister://")
	anonymousRec := httptest.NewRecorder()

	handler.ServeHTTP(anonymousRec, anonymousReq)

	if anonymousRec.Code != http.StatusForbidden {
		t.Fatalf("anonymous POST /api/history status = %d, want %d", anonymousRec.Code, http.StatusForbidden)
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	readReq.Header.Set("X-Access-Token", "secret")
	readRec := httptest.NewRecorder()

	handler.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusNotFound {
		t.Fatalf("authenticated GET /api/history status = %d, want %d", readRec.Code, http.StatusNotFound)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/history", strings.NewReader(`{"query":"q","url":"https://example.com"}`))
	req.Header.Set("Origin", "hister://")
	req.Header.Set("X-Access-Token", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/history status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestTokenAuthStillProtectsPublicRoutesWhenPublicModeDisabled(t *testing.T) {
	_, handler := newTokenTestServer(t, false)
	req := httptest.NewRequest(http.MethodGet, "/search?format=json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /search status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
