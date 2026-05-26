package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mergerhq/merger/internal/config"
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
