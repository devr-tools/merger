package access_test

import (
	"context"
	"errors"
	"strings"
	"testing"

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
