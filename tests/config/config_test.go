package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/config"
)

func TestDefaults(t *testing.T) {
	cfg := config.Defaults()

	if cfg.Service.IngestAddress != ":8080" {
		t.Fatalf("unexpected ingest address: %q", cfg.Service.IngestAddress)
	}
	if cfg.Service.ControlPlaneAddress != ":8081" {
		t.Fatalf("unexpected control plane address: %q", cfg.Service.ControlPlaneAddress)
	}
	if cfg.Service.ControlPlaneGRPC != ":9091" {
		t.Fatalf("unexpected control plane grpc address: %q", cfg.Service.ControlPlaneGRPC)
	}
	if cfg.Policy.Path == "" {
		t.Fatal("expected default policy path")
	}
	if cfg.Access.Mode != config.AccessModeDisabled {
		t.Fatalf("expected access to default to disabled, got %q", cfg.Access.Mode)
	}
}

func TestLoadStaticTokenAccessConfiguration(t *testing.T) {
	configPath := writeConfig(t, `
access:
  mode: static_token
  tokens:
    - subject: ci
      token_env: MERGER_CI_TOKEN
      roles: [reader, evidence_writer]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Access.Mode != config.AccessModeStaticToken || len(cfg.Access.Tokens) != 1 {
		t.Fatalf("unexpected access config: %#v", cfg.Access)
	}
	token := cfg.Access.Tokens[0]
	if token.Subject != "ci" || token.TokenEnv != "MERGER_CI_TOKEN" {
		t.Fatalf("unexpected token reference: %#v", token)
	}
	if len(token.Roles) != 2 || token.Roles[1] != access.RoleEvidenceWriter {
		t.Fatalf("unexpected token roles: %#v", token.Roles)
	}
}

func TestValidateRejectsInvalidAccessConfiguration(t *testing.T) {
	validToken := config.AccessTokenConfig{
		Subject: "ci", TokenEnv: "MERGER_CI_TOKEN", Roles: []access.Role{access.RoleEvidenceWriter},
	}
	tests := []struct {
		name    string
		access  config.AccessConfig
		wantErr string
	}{
		{
			name:    "unsupported mode",
			access:  config.AccessConfig{Mode: "oauth"},
			wantErr: "unsupported access mode",
		},
		{
			name:    "disabled with tokens",
			access:  config.AccessConfig{Mode: config.AccessModeDisabled, Tokens: []config.AccessTokenConfig{validToken}},
			wantErr: "must be empty",
		},
		{
			name:    "static without tokens",
			access:  config.AccessConfig{Mode: config.AccessModeStaticToken},
			wantErr: "at least one entry",
		},
		{
			name: "duplicate subject",
			access: config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{
				validToken,
				{Subject: "CI", TokenEnv: "MERGER_OTHER_TOKEN", Roles: []access.Role{access.RoleReader}},
			}},
			wantErr: "subject \"CI\" is duplicated",
		},
		{
			name: "duplicate environment",
			access: config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{
				validToken,
				{Subject: "dashboard", TokenEnv: "MERGER_CI_TOKEN", Roles: []access.Role{access.RoleReader}},
			}},
			wantErr: "environment variable \"MERGER_CI_TOKEN\" is duplicated",
		},
		{
			name: "invalid environment",
			access: config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{
				{Subject: "ci", TokenEnv: "1-BAD", Roles: []access.Role{access.RoleReader}},
			}},
			wantErr: "not a valid environment variable name",
		},
		{
			name: "unsupported role",
			access: config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{
				{Subject: "ci", TokenEnv: "MERGER_CI_TOKEN", Roles: []access.Role{"owner"}},
			}},
			wantErr: "unsupported role",
		},
		{
			name: "empty roles",
			access: config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{
				{Subject: "ci", TokenEnv: "MERGER_CI_TOKEN"},
			}},
			wantErr: "at least one role",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Access = test.access
			err := config.Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	configPath := writeConfig(t, `
lanes:
  green_max: 20
  yellow_max: 55
  red_max: 85
  purple_max: 95
`)

	_, err := config.Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "purple_max") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestValidateRejectsInvalidThresholdsAndBackends(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{
			name: "lane out of range",
			mutate: func(cfg *config.Config) {
				cfg.Lanes.RedMax = 101
			},
			wantErr: "between 0 and 100",
		},
		{
			name: "lane ordering",
			mutate: func(cfg *config.Config) {
				cfg.Lanes.YellowMax = cfg.Lanes.GreenMax
			},
			wantErr: "strictly increasing",
		},
		{
			name: "events backend",
			mutate: func(cfg *config.Config) {
				cfg.Events.Backend = "redis"
			},
			wantErr: "unsupported events backend",
		},
		{
			name: "persistence backend",
			mutate: func(cfg *config.Config) {
				cfg.Persistence.Backend = "sqlite"
			},
			wantErr: "unsupported persistence backend",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Defaults()
			test.mutate(&cfg)
			err := config.Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestValidateForIngestRequiresGitHubSettingsWhenEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.GitHub.Enabled = true
	cfg.GitHub.AppID = "replace-me"

	err := config.ValidateForIngest(cfg)
	if err == nil {
		t.Fatal("expected missing GitHub settings error")
	}
	for _, field := range []string{"github.webhook_secret", "github.app_id", "github.private_key_path"} {
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("expected error to mention %s, got %v", field, err)
		}
	}

	cfg.GitHub.WebhookSecret = "secret"
	cfg.GitHub.AppID = "1234"
	cfg.GitHub.PrivateKeyPath = "/run/secrets/github-app.pem"
	if err := config.ValidateForIngest(cfg); err != nil {
		t.Fatalf("expected complete GitHub configuration to be valid, got %v", err)
	}
}

func TestValidateForServiceDoesNotRequireUnusedGitHubCredentials(t *testing.T) {
	cfg := config.Defaults()
	cfg.GitHub.Enabled = true
	if err := config.ValidateForService(cfg); err != nil {
		t.Fatalf("expected shared service validation not to require ingest credentials, got %v", err)
	}
}

func TestValidateForControlPlaneRejectsDisabledProductionAccess(t *testing.T) {
	cfg := config.Defaults()
	cfg.Telemetry.Environment = "production"

	err := config.ValidateForControlPlane(cfg)
	if err == nil || !strings.Contains(err.Error(), "access.mode") {
		t.Fatalf("expected production access validation error, got %v", err)
	}

	cfg.Access = config.AccessConfig{Mode: config.AccessModeStaticToken, Tokens: []config.AccessTokenConfig{{
		Subject:  "operator",
		TokenEnv: "MERGER_OPERATOR_TOKEN",
		Roles:    []access.Role{access.RoleAdmin},
	}}}
	if err := config.ValidateForControlPlane(cfg); err != nil {
		t.Fatalf("expected production static-token access to be valid, got %v", err)
	}
}

func TestLoadOverridesConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "merger.yaml")

	err := os.WriteFile(configPath, []byte(`
service:
  ingest_address: ":18080"
  controlplane_address: ":18081"
  controlplane_grpc_address: ":19091"
github:
  enabled: true
events:
  backend: memory
lanes:
  green_max: 10
  yellow_max: 40
  red_max: 90
`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Service.IngestAddress != ":18080" {
		t.Fatalf("unexpected ingest address: %q", cfg.Service.IngestAddress)
	}
	if cfg.Service.ControlPlaneAddress != ":18081" {
		t.Fatalf("unexpected control plane address: %q", cfg.Service.ControlPlaneAddress)
	}
	if cfg.Service.ControlPlaneGRPC != ":19091" {
		t.Fatalf("unexpected control plane grpc address: %q", cfg.Service.ControlPlaneGRPC)
	}
	if !cfg.GitHub.Enabled {
		t.Fatal("expected github integration to be enabled")
	}
	if cfg.Lanes.GreenMax != 10 || cfg.Lanes.YellowMax != 40 || cfg.Lanes.RedMax != 90 {
		t.Fatalf("unexpected lane thresholds: %+v", cfg.Lanes)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "merger.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
