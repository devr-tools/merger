package main

import (
	"context"
	"log"
	"os"

	ingestapp "github.com/devr-tools/merger/internal/app/ingest"
	"github.com/devr-tools/merger/internal/bootstrap"
	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/telemetry"
)

func main() {
	configPath := os.Getenv("MERGER_CONFIG_PATH")
	if configPath == "" {
		configPath = "config/merger.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := config.ValidateForIngest(cfg); err != nil {
		log.Fatal(err)
	}

	policyConfig, err := policy.LoadConfig(cfg.Policy.Path)
	if err != nil {
		log.Fatal(err)
	}

	logger := telemetry.NewLogger(cfg.Logging.Level)
	repo, err := bootstrap.BuildRepository(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()

	bus, err := bootstrap.BuildEventBus(cfg, repo)
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	githubService, err := bootstrap.BuildGitHubService(cfg)
	if err != nil {
		log.Fatal(err)
	}

	app := ingestapp.New(
		cfg,
		logger,
		telemetry.NewTracer(),
		bus,
		githubService,
		policy.NewRuleEngine(policyConfig),
		repo,
	)

	log.Fatal(app.Run())
}
