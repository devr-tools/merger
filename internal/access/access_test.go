package access_test

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/access"
)

func TestStaticTokenAuthenticatorAuthenticatesEnvironmentToken(t *testing.T) {
	t.Setenv("MERGER_READER_TOKEN", "reader-secret")
	t.Setenv("MERGER_WRITER_TOKEN", "writer-secret")

	authenticator, err := access.NewStaticTokenAuthenticator([]access.StaticToken{
		{Subject: "dashboard", TokenEnv: "MERGER_READER_TOKEN", Roles: []access.Role{access.RoleReader}},
		{Subject: "ci", TokenEnv: "MERGER_WRITER_TOKEN", Roles: []access.Role{access.RoleReader, access.RoleEvidenceWriter}},
	})
	if err != nil {
		t.Fatalf("construct authenticator: %v", err)
	}

	principal, err := authenticator.Authenticate("bearer writer-secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.Subject != "ci" {
		t.Fatalf("expected ci principal, got %q", principal.Subject)
	}
	if !principal.HasRole(access.RoleReader) || !principal.HasRole(access.RoleEvidenceWriter) {
		t.Fatalf("expected reader and evidence-writer roles, got %#v", principal.Roles)
	}
}

func TestStaticTokenAuthenticatorRejectsInvalidBearerCredentials(t *testing.T) {
	t.Setenv("MERGER_ACCESS_TOKEN", "correct-secret")
	authenticator, err := access.NewStaticTokenAuthenticator([]access.StaticToken{{
		Subject: "client", TokenEnv: "MERGER_ACCESS_TOKEN", Roles: []access.Role{access.RoleReader},
	}})
	if err != nil {
		t.Fatalf("construct authenticator: %v", err)
	}

	for _, authorization := range []string{"", "Basic correct-secret", "Bearer", "Bearer wrong-secret", "Bearer two words"} {
		t.Run(authorization, func(t *testing.T) {
			_, err := authenticator.Authenticate(authorization)
			if !errors.Is(err, access.ErrUnauthenticated) {
				t.Fatalf("expected unauthenticated for %q, got %v", authorization, err)
			}
		})
	}
}

func TestStaticTokenAuthenticatorFailsClosedForMissingEnvironmentSecret(t *testing.T) {
	const secret = "must-not-appear"
	_, err := access.NewStaticTokenAuthenticator([]access.StaticToken{{
		Subject: "client", TokenEnv: "MERGER_MISSING_TOKEN", Roles: []access.Role{access.RoleReader},
	}})
	if err == nil || !strings.Contains(err.Error(), "MERGER_MISSING_TOKEN") {
		t.Fatalf("expected missing environment error, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed a token value: %v", err)
	}
}

func TestContextAndAuthorization(t *testing.T) {
	roles := []access.Role{access.RoleEvidenceWriter}
	ctx := access.WithPrincipal(context.Background(), access.Principal{Subject: "ci", Roles: roles})
	roles[0] = access.RoleAdmin

	principal, ok := access.PrincipalFromContext(ctx)
	if !ok || principal.HasRole(access.RoleAdmin) {
		t.Fatalf("expected context to retain a defensive role copy, got %#v", principal)
	}
	principal.Roles[0] = access.RoleAdmin
	stored, _ := access.PrincipalFromContext(ctx)
	if stored.HasRole(access.RoleAdmin) {
		t.Fatal("mutating returned principal changed context authorization state")
	}

	if err := access.RequireRole(ctx, access.RoleEvidenceWriter); err != nil {
		t.Fatalf("expected evidence writer authorization, got %v", err)
	}
	if err := access.RequireRole(ctx, access.RoleReader); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("expected reader authorization to be forbidden, got %v", err)
	}
	if err := access.RequireRole(context.Background(), access.RoleReader); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("expected missing principal to be unauthenticated, got %v", err)
	}

	admin := access.Principal{Subject: "operator", Roles: []access.Role{access.RoleAdmin}}
	if err := access.Authorize(admin, access.RoleEvidenceWriter); err != nil {
		t.Fatalf("expected admin override, got %v", err)
	}
}

func TestDisabledAuthenticatorReturnsLocalAdministrator(t *testing.T) {
	var authenticator access.Authenticator = access.NewDisabledAuthenticator()
	principal, err := authenticator.Authenticate("")
	if err != nil {
		t.Fatalf("authenticate with disabled mode: %v", err)
	}
	if principal.Subject != "local" || !principal.HasRole(access.RoleAdmin) {
		t.Fatalf("expected local administrator, got %#v", principal)
	}
}

func TestJWTAuthenticatorAuthenticatesHS256Token(t *testing.T) {
	t.Setenv("MERGER_TEST_JWT_SECRET", "jwt-secret")
	authenticator, err := access.NewJWTAuthenticator(access.JWTConfig{
		Algorithm:  access.JWTAlgorithmHS256,
		Issuer:     "https://auth.example.test",
		Audience:   "merger-controlplane",
		SecretEnv:  "MERGER_TEST_JWT_SECRET",
		RolesClaim: "scope",
		RoleBindings: []access.JWTClaimBinding{
			{ClaimValue: "merger.read", Roles: []access.Role{access.RoleReader}},
			{ClaimValue: "merger.write", Roles: []access.Role{access.RoleEvidenceWriter}},
		},
	})
	if err != nil {
		t.Fatalf("construct jwt authenticator: %v", err)
	}

	token := signedHS256JWT(t, "jwt-secret", map[string]any{
		"iss":   "https://auth.example.test",
		"aud":   "merger-controlplane",
		"sub":   "ci-workflow",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "merger.read merger.write",
	})

	principal, err := authenticator.Authenticate("Bearer " + token)
	if err != nil {
		t.Fatalf("authenticate jwt: %v", err)
	}
	if principal.Subject != "ci-workflow" {
		t.Fatalf("expected ci-workflow principal, got %q", principal.Subject)
	}
	if !principal.HasRole(access.RoleReader) || !principal.HasRole(access.RoleEvidenceWriter) {
		t.Fatalf("expected mapped roles, got %#v", principal.Roles)
	}
}

func TestJWTAuthenticatorAuthenticatesRS256Token(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "jwt-public.pem")
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	}), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	authenticator, err := access.NewJWTAuthenticator(access.JWTConfig{
		Algorithm:     access.JWTAlgorithmRS256,
		Issuer:        "https://auth.example.test",
		Audience:      "merger-controlplane",
		PublicKeyPath: publicKeyPath,
		RoleBindings: []access.JWTClaimBinding{
			{ClaimValue: "merger.admin", Roles: []access.Role{access.RoleAdmin}},
		},
	})
	if err != nil {
		t.Fatalf("construct rs256 authenticator: %v", err)
	}

	token := signedRS256JWT(t, privateKey, map[string]any{
		"iss":   "https://auth.example.test",
		"aud":   []string{"merger-controlplane"},
		"sub":   "operator",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []string{"merger.admin"},
	})

	principal, err := authenticator.Authenticate("Bearer " + token)
	if err != nil {
		t.Fatalf("authenticate rs256 jwt: %v", err)
	}
	if principal.Subject != "operator" || !principal.HasRole(access.RoleAdmin) {
		t.Fatalf("expected operator admin principal, got %#v", principal)
	}
}

func TestJWTAuthenticatorRejectsInvalidClaimsAndSignatures(t *testing.T) {
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

	tests := []struct {
		name  string
		token string
	}{
		{
			name: "wrong issuer",
			token: signedHS256JWT(t, "jwt-secret", map[string]any{
				"iss":   "https://other.example.test",
				"aud":   "merger-controlplane",
				"sub":   "dashboard",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"roles": []string{"merger.read"},
			}),
		},
		{
			name: "expired",
			token: signedHS256JWT(t, "jwt-secret", map[string]any{
				"iss":   "https://auth.example.test",
				"aud":   "merger-controlplane",
				"sub":   "dashboard",
				"exp":   time.Now().Add(-time.Minute).Unix(),
				"roles": []string{"merger.read"},
			}),
		},
		{
			name: "bad signature",
			token: signedHS256JWT(t, "other-secret", map[string]any{
				"iss":   "https://auth.example.test",
				"aud":   "merger-controlplane",
				"sub":   "dashboard",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"roles": []string{"merger.read"},
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := authenticator.Authenticate("Bearer " + test.token)
			if !errors.Is(err, access.ErrUnauthenticated) {
				t.Fatalf("expected unauthenticated error, got %v", err)
			}
		})
	}
}

func signedHS256JWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()

	header := encodeJWTPart(t, map[string]any{"alg": "HS256", "typ": "JWT"})
	payload := encodeJWTPart(t, claims)
	signingInput := header + "." + payload

	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signingInput)); err != nil {
		t.Fatalf("sign hs256 jwt: %v", err)
	}
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func signedRS256JWT(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()

	header := encodeJWTPart(t, map[string]any{"alg": "RS256", "typ": "JWT"})
	payload := encodeJWTPart(t, claims)
	signingInput := header + "." + payload

	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign rs256 jwt: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func encodeJWTPart(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal jwt part: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
