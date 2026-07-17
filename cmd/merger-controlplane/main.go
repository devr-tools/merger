package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/devr-tools/merger/internal/access"
	controlplaneapp "github.com/devr-tools/merger/internal/app/controlplane"
	"github.com/devr-tools/merger/internal/bootstrap"
	"github.com/devr-tools/merger/internal/checks"
	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/lanes"
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
	if err := config.ValidateForControlPlane(cfg); err != nil {
		log.Fatal(err)
	}

	logger := telemetry.NewLogger(cfg.Logging.Level)
	authenticator, err := buildAuthenticator(cfg.Access)
	if err != nil {
		log.Fatal(err)
	}

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

	app := controlplaneapp.New(
		cfg,
		logger,
		bus,
		repo,
		authenticator,
		controlplane.WithLaneAssigner(lanes.NewAssigner(lanes.Config{
			GreenMax:  cfg.Lanes.GreenMax,
			YellowMax: cfg.Lanes.YellowMax,
			RedMax:    cfg.Lanes.RedMax,
		})),
		controlplane.WithCheckPublisher(checks.NewGitHubCheckPublisher(githubService)),
	)

	log.Fatal(app.Run())
}

func buildAuthenticator(cfg config.AccessConfig) (access.Authenticator, error) {
	switch cfg.Mode {
	case config.AccessModeDisabled:
		return access.NewDisabledAuthenticator(), nil
	case config.AccessModeStaticToken:
		tokens := make([]access.StaticToken, 0, len(cfg.Tokens))
		for _, token := range cfg.Tokens {
			tokens = append(tokens, access.StaticToken{
				Subject:  token.Subject,
				TokenEnv: token.TokenEnv,
				Roles:    token.Roles,
			})
		}
		return access.NewStaticTokenAuthenticator(tokens)
	default:
		return nil, fmt.Errorf("unsupported access mode %q", cfg.Mode)
	}
}
