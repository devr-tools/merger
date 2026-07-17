package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/config"
)

func TestBuildAuthenticator(t *testing.T) {
	t.Run("disabled grants local admin", func(t *testing.T) {
		authenticator, err := buildAuthenticator(config.AccessConfig{Mode: config.AccessModeDisabled})
		if err != nil {
			t.Fatalf("build disabled authenticator: %v", err)
		}
		principal, err := authenticator.Authenticate("")
		if err != nil || !principal.HasRole(access.RoleAdmin) {
			t.Fatalf("expected local admin, got principal=%#v err=%v", principal, err)
		}
	})

	t.Run("static token uses configured environment", func(t *testing.T) {
		t.Setenv("MERGER_CONTROLPLANE_TEST_TOKEN", "reader-secret")
		authenticator, err := buildAuthenticator(config.AccessConfig{
			Mode: config.AccessModeStaticToken,
			Tokens: []config.AccessTokenConfig{{
				Subject:  "dashboard",
				TokenEnv: "MERGER_CONTROLPLANE_TEST_TOKEN",
				Roles:    []access.Role{access.RoleReader},
			}},
		})
		if err != nil {
			t.Fatalf("build static authenticator: %v", err)
		}
		principal, err := authenticator.Authenticate("Bearer reader-secret")
		if err != nil || principal.Subject != "dashboard" || !principal.HasRole(access.RoleReader) {
			t.Fatalf("unexpected principal=%#v err=%v", principal, err)
		}
	})

	t.Run("jwt uses configured issuer and secret", func(t *testing.T) {
		t.Setenv("MERGER_CONTROLPLANE_TEST_JWT_SECRET", "top-secret")
		authenticator, err := buildAuthenticator(config.AccessConfig{
			Mode: config.AccessModeJWT,
			JWT: config.AccessJWTConfig{
				Algorithm: "HS256",
				Issuer:    "https://auth.example.test",
				Audience:  "merger-controlplane",
				SecretEnv: "MERGER_CONTROLPLANE_TEST_JWT_SECRET",
				RoleBindings: []config.AccessJWTBindingConfig{
					{ClaimValue: "merger.read", Roles: []access.Role{access.RoleReader}},
				},
			},
		})
		if err != nil {
			t.Fatalf("build jwt authenticator: %v", err)
		}
		token := signedHS256Token(t, "top-secret", map[string]any{
			"iss":   "https://auth.example.test",
			"aud":   "merger-controlplane",
			"sub":   "dashboard",
			"exp":   time.Now().Add(time.Hour).Unix(),
			"roles": []string{"merger.read"},
		})
		principal, err := authenticator.Authenticate("Bearer " + token)
		if err != nil || principal.Subject != "dashboard" || !principal.HasRole(access.RoleReader) {
			t.Fatalf("unexpected principal=%#v err=%v", principal, err)
		}
	})
}

func signedHS256Token(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()

	headerSegment := encodeJWTComponent(t, map[string]any{"alg": "HS256", "typ": "JWT"})
	claimsSegment := encodeJWTComponent(t, claims)
	signingInput := headerSegment + "." + claimsSegment

	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signingInput)); err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	signatureSegment := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signatureSegment
}

func encodeJWTComponent(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal jwt component: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
