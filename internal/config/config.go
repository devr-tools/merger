package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Service      ServiceConfig      `yaml:"service"`
	Logging      LoggingConfig      `yaml:"logging"`
	GitHub       GitHubConfig       `yaml:"github"`
	Events       EventsConfig       `yaml:"events"`
	Persistence  PersistenceConfig  `yaml:"persistence"`
	Policy       PolicyConfig       `yaml:"policy"`
	Lanes        LanesConfig        `yaml:"lanes"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	RuntimeGraph RuntimeGraphConfig `yaml:"runtime_graph"`
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

	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
