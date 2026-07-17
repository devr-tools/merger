package controlplane_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/controlplane"
)

func TestAccessMiddlewareEnforcesControlPlaneRoles(t *testing.T) {
	t.Setenv("MERGER_TEST_READER_TOKEN", "reader-secret")
	t.Setenv("MERGER_TEST_WRITER_TOKEN", "writer-secret")
	t.Setenv("MERGER_TEST_ADMIN_TOKEN", "admin-secret")
	authenticator, err := access.NewStaticTokenAuthenticator([]access.StaticToken{
		{Subject: "dashboard", TokenEnv: "MERGER_TEST_READER_TOKEN", Roles: []access.Role{access.RoleReader}},
		{Subject: "ci", TokenEnv: "MERGER_TEST_WRITER_TOKEN", Roles: []access.Role{access.RoleEvidenceWriter}},
		{Subject: "operator", TokenEnv: "MERGER_TEST_ADMIN_TOKEN", Roles: []access.Role{access.RoleAdmin}},
	})
	if err != nil {
		t.Fatalf("construct authenticator: %v", err)
	}

	repo := seedRepository(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	controlplane.NewHTTPHandler(controlplane.NewService(repo)).Register(mux)
	handler := controlplane.AccessMiddleware(authenticator, mux)

	tests := []struct {
		name          string
		method        string
		path          string
		authorization string
		body          string
		wantStatus    int
		wantChallenge bool
	}{
		{name: "health is public", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "missing credentials", method: http.MethodGet, path: "/api/v1/change-packets", wantStatus: http.StatusUnauthorized, wantChallenge: true},
		{name: "invalid credentials", method: http.MethodGet, path: "/api/v1/change-packets", authorization: "Bearer wrong-secret", wantStatus: http.StatusUnauthorized, wantChallenge: true},
		{name: "writer cannot read", method: http.MethodGet, path: "/api/v1/change-packets/cp_1", authorization: "Bearer writer-secret", wantStatus: http.StatusForbidden},
		{name: "reader can read", method: http.MethodGet, path: "/api/v1/change-packets/cp_1", authorization: "Bearer reader-secret", wantStatus: http.StatusOK},
		{name: "reader cannot update evidence", method: http.MethodPut, path: "/api/v1/change-packets/cp_1/evidence/integration_tests", authorization: "Bearer reader-secret", body: `{"status":"running"}`, wantStatus: http.StatusForbidden},
		{name: "writer can update evidence", method: http.MethodPut, path: "/api/v1/change-packets/cp_1/evidence/integration_tests", authorization: "Bearer writer-secret", body: `{"status":"running"}`, wantStatus: http.StatusAccepted},
		{name: "admin can read", method: http.MethodGet, path: "/api/v1/change-packets/cp_1", authorization: "Bearer admin-secret", wantStatus: http.StatusOK},
		{name: "admin can update evidence", method: http.MethodPut, path: "/api/v1/change-packets/cp_1/evidence/integration_tests", authorization: "Bearer admin-secret", body: `{"status":"satisfied"}`, wantStatus: http.StatusAccepted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			if test.authorization != "" {
				req.Header.Set("Authorization", test.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d: %s", test.wantStatus, rec.Code, rec.Body.String())
			}
			if test.wantChallenge && rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("expected Bearer challenge, got %q", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestAccessMiddlewareAllowsDisabledModeWithoutCredentials(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := controlplane.AccessMiddleware(access.NewDisabledAuthenticator(), next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected disabled access mode to allow local request, got %d", rec.Code)
	}
}

func TestAccessMiddlewareAcceptsJWTBearerToken(t *testing.T) {
	t.Setenv("MERGER_TEST_JWT_SECRET", "jwt-secret")
	authenticator, err := access.NewJWTAuthenticator(access.JWTConfig{
		Algorithm: access.JWTAlgorithmHS256,
		Issuer:    "https://auth.example.test",
		Audience:  "merger-controlplane",
		SecretEnv: "MERGER_TEST_JWT_SECRET",
		RoleBindings: []access.JWTClaimBinding{
			{ClaimValue: "merger.read", Roles: []access.Role{access.RoleReader}},
		},
	})
	if err != nil {
		t.Fatalf("construct jwt authenticator: %v", err)
	}

	repo := seedRepository(t)
	mux := http.NewServeMux()
	controlplane.NewHTTPHandler(controlplane.NewService(repo)).Register(mux)
	handler := controlplane.AccessMiddleware(authenticator, mux)

	token := signedJWT(t, "jwt-secret", map[string]any{
		"iss":   "https://auth.example.test",
		"aud":   "merger-controlplane",
		"sub":   "dashboard",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []string{"merger.read"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets/cp_1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected jwt-authenticated request to succeed, got %d: %s", rec.Code, rec.Body.String())
	}
}

func signedJWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()

	header := encodeJWTComponent(t, map[string]any{"alg": "HS256", "typ": "JWT"})
	payload := encodeJWTComponent(t, claims)
	signingInput := header + "." + payload

	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signingInput)); err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeJWTComponent(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal jwt component: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
