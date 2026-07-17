package main

import (
	"testing"

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
}
