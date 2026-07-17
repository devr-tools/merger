package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/devr-tools/merger/internal/access"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Service      ServiceConfig      `yaml:"service"`
	Access       AccessConfig       `yaml:"access"`
	Logging      LoggingConfig      `yaml:"logging"`
	GitHub       GitHubConfig       `yaml:"github"`
	Events       EventsConfig       `yaml:"events"`
	Persistence  PersistenceConfig  `yaml:"persistence"`
	Policy       PolicyConfig       `yaml:"policy"`
	Lanes        LanesConfig        `yaml:"lanes"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	RuntimeGraph RuntimeGraphConfig `yaml:"runtime_graph"`
}

type AccessMode string

const (
	AccessModeDisabled    AccessMode = "disabled"
	AccessModeStaticToken AccessMode = "static_token"
)

type AccessConfig struct {
	Mode   AccessMode          `yaml:"mode"`
	Tokens []AccessTokenConfig `yaml:"tokens"`
}

type AccessTokenConfig struct {
	Subject  string        `yaml:"subject"`
	TokenEnv string        `yaml:"token_env"`
	Roles    []access.Role `yaml:"roles"`
}

type ServiceConfig struct {
	IngestAddress       string `yaml:"ingest_address"`
	ControlPlaneAddress string `yaml:"controlplane_address"`
	ControlPlaneGRPC    string `yaml:"controlplane_grpc_address"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type GitHubConfig struct {
	Enabled        bool   `yaml:"enabled"`
	WebhookSecret  string `yaml:"webhook_secret"`
	AppID          string `yaml:"app_id"`
	InstallationID int64  `yaml:"installation_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	APIBaseURL     string `yaml:"api_base_url"`
	Timeout        string `yaml:"timeout"`
}

type EventsConfig struct {
	Backend       string `yaml:"backend"`
	SubjectPrefix string `yaml:"subject_prefix"`
	NATSURL       string `yaml:"nats_url"`
	StreamName    string `yaml:"stream_name"`
	DurablePrefix string `yaml:"durable_prefix"`
}

type PersistenceConfig struct {
	Backend     string `yaml:"backend"`
	DatabaseURL string `yaml:"database_url"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}

type PolicyConfig struct {
	Path string `yaml:"path"`
}

type LanesConfig struct {
	GreenMax  int `yaml:"green_max"`
	YellowMax int `yaml:"yellow_max"`
	RedMax    int `yaml:"red_max"`
}

type TelemetryConfig struct {
	ServiceName string `yaml:"service_name"`
	Environment string `yaml:"environment"`
}

type RuntimeGraphConfig struct {
	EnableCodeOwners bool `yaml:"enable_codeowners"`
}

func Defaults() Config {
	return Config{
		Service: ServiceConfig{
			IngestAddress:       ":8080",
			ControlPlaneAddress: ":8081",
			ControlPlaneGRPC:    ":9091",
		},
		Access:  AccessConfig{Mode: AccessModeDisabled},
		Logging: LoggingConfig{Level: "info"},
		GitHub: GitHubConfig{
			APIBaseURL: "https://api.github.com",
			Timeout:    "10s",
		},
		Events: EventsConfig{
			Backend:       "memory",
			SubjectPrefix: "merger",
			NATSURL:       "nats://127.0.0.1:4222",
			StreamName:    "MERGER_EVENTS",
			DurablePrefix: "merger",
		},
		Persistence: PersistenceConfig{
			Backend:     "memory",
			DatabaseURL: "postgres://merger:merger@127.0.0.1:5432/merger?sslmode=disable",
			AutoMigrate: true,
		},
		Policy: PolicyConfig{
			Path: "config/policies/default.yaml",
		},
		Lanes: LanesConfig{
			GreenMax:  20,
			YellowMax: 55,
			RedMax:    85,
		},
		Telemetry: TelemetryConfig{
			ServiceName: "merger",
			Environment: "dev",
		},
		RuntimeGraph: RuntimeGraphConfig{
			EnableCodeOwners: true,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if err := ensureSingleDocument(decoder); err != nil {
		return Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate checks configuration values that apply to every merger execution,
// including local scans and long-running services.
func Validate(cfg Config) error {
	if err := validateAccess(cfg.Access); err != nil {
		return err
	}

	if cfg.Lanes.GreenMax < 0 || cfg.Lanes.GreenMax > 100 ||
		cfg.Lanes.YellowMax < 0 || cfg.Lanes.YellowMax > 100 ||
		cfg.Lanes.RedMax < 0 || cfg.Lanes.RedMax > 100 {
		return fmt.Errorf("lane thresholds must be between 0 and 100: green(%d), yellow(%d), red(%d)", cfg.Lanes.GreenMax, cfg.Lanes.YellowMax, cfg.Lanes.RedMax)
	}
	if !(cfg.Lanes.GreenMax < cfg.Lanes.YellowMax && cfg.Lanes.YellowMax < cfg.Lanes.RedMax) {
		return fmt.Errorf("lane thresholds must be strictly increasing: green(%d) < yellow(%d) < red(%d)", cfg.Lanes.GreenMax, cfg.Lanes.YellowMax, cfg.Lanes.RedMax)
	}

	if !oneOf(cfg.Events.Backend, "memory", "nats") {
		return fmt.Errorf("unsupported events backend %q (supported: memory, nats)", cfg.Events.Backend)
	}
	if !oneOf(cfg.Persistence.Backend, "memory", "postgres") {
		return fmt.Errorf("unsupported persistence backend %q (supported: memory, postgres)", cfg.Persistence.Backend)
	}

	return nil
}

func validateAccess(cfg AccessConfig) error {
	switch cfg.Mode {
	case AccessModeDisabled:
		if len(cfg.Tokens) > 0 {
			return fmt.Errorf("access tokens must be empty when access.mode is %q", AccessModeDisabled)
		}
		return nil
	case AccessModeStaticToken:
		if len(cfg.Tokens) == 0 {
			return fmt.Errorf("access.tokens must contain at least one entry when access.mode is %q", AccessModeStaticToken)
		}
	default:
		return fmt.Errorf("unsupported access mode %q (supported: disabled, static_token)", cfg.Mode)
	}

	subjects := make(map[string]struct{}, len(cfg.Tokens))
	environments := make(map[string]struct{}, len(cfg.Tokens))
	for index, token := range cfg.Tokens {
		subject := strings.TrimSpace(token.Subject)
		if subject == "" {
			return fmt.Errorf("access.tokens[%d].subject must not be empty", index)
		}
		subjectKey := strings.ToLower(subject)
		if _, duplicate := subjects[subjectKey]; duplicate {
			return fmt.Errorf("access token subject %q is duplicated", subject)
		}
		subjects[subjectKey] = struct{}{}

		tokenEnv := strings.TrimSpace(token.TokenEnv)
		if !validEnvironmentName(tokenEnv) {
			return fmt.Errorf("access.tokens[%d].token_env %q is not a valid environment variable name", index, token.TokenEnv)
		}
		if _, duplicate := environments[tokenEnv]; duplicate {
			return fmt.Errorf("access token environment variable %q is duplicated", tokenEnv)
		}
		environments[tokenEnv] = struct{}{}

		if len(token.Roles) == 0 {
			return fmt.Errorf("access.tokens[%d].roles must contain at least one role", index)
		}
		roles := make(map[access.Role]struct{}, len(token.Roles))
		for _, role := range token.Roles {
			if !access.IsSupportedRole(role) {
				return fmt.Errorf("access.tokens[%d] has unsupported role %q", index, role)
			}
			if _, duplicate := roles[role]; duplicate {
				return fmt.Errorf("access.tokens[%d] has duplicate role %q", index, role)
			}
			roles[role] = struct{}{}
		}
	}

	return nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' {
			continue
		}
		if index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

// ValidateForService checks configuration shared by long-running services.
func ValidateForService(cfg Config) error {
	return Validate(cfg)
}

// ValidateForControlPlane adds checks required by the control-plane service.
func ValidateForControlPlane(cfg Config) error {
	if err := ValidateForGitHubIntegration(cfg); err != nil {
		return err
	}
	environment := strings.ToLower(strings.TrimSpace(cfg.Telemetry.Environment))
	if cfg.Access.Mode == AccessModeDisabled && (environment == "prod" || environment == "production") {
		return fmt.Errorf("access.mode must be %q in production", AccessModeStaticToken)
	}
	return nil
}

// ValidateForIngest checks integrations constructed by the ingest service.
func ValidateForIngest(cfg Config) error {
	return ValidateForGitHubIntegration(cfg)
}

// ValidateForGitHubIntegration adds checks required by services that construct
// the GitHub client. InstallationID may be zero because webhook deliveries and
// persisted Change Packets bind their GitHub App installation dynamically.
func ValidateForGitHubIntegration(cfg Config) error {
	if err := ValidateForService(cfg); err != nil {
		return err
	}
	if !cfg.GitHub.Enabled {
		return nil
	}

	missing := make([]string, 0, 3)
	if missingOrPlaceholder(cfg.GitHub.WebhookSecret) {
		missing = append(missing, "github.webhook_secret")
	}
	if missingOrPlaceholder(cfg.GitHub.AppID) {
		missing = append(missing, "github.app_id")
	}
	if missingOrPlaceholder(cfg.GitHub.PrivateKeyPath) {
		missing = append(missing, "github.private_key_path")
	}
	if len(missing) > 0 {
		return fmt.Errorf("github is enabled but required settings are missing or invalid: %s", strings.Join(missing, ", "))
	}

	return nil
}

func missingOrPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "replace-me", "replace_me", "change-me", "changeme", "todo":
		return true
	default:
		return false
	}
}

func ensureSingleDocument(decoder *yaml.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("configuration must contain exactly one YAML document")
		}
		return err
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
