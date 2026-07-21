package ingestapp

import (
	"context"
	"net/http"

	"github.com/devr-tools/merger/internal/checks"
	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/ingest"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/risk"
	"github.com/devr-tools/merger/internal/runtimegraph"
	"github.com/devr-tools/merger/internal/store"
	"github.com/devr-tools/merger/internal/telemetry"
)

type App struct {
	server *http.Server
}

func New(
	cfg config.Config,
	logger *telemetry.Logger,
	tracer telemetry.Tracer,
	bus events.Bus,
	githubService github.Service,
	policyEngine policy.Engine,
	repository store.Repository,
) *App {
	checkPublisher := checks.NewGitHubCheckPublisher(githubService)
	processor := ingest.NewProcessor(
		logger,
		tracer,
		bus,
		githubService,
		mutations.DefaultEngine(),
		risk.DefaultEngine(),
		policyEngine,
		lanes.NewAssigner(lanes.Config{
			GreenMax:  cfg.Lanes.GreenMax,
			YellowMax: cfg.Lanes.YellowMax,
			RedMax:    cfg.Lanes.RedMax,
		}),
		checkPublisher,
		runtimegraph.NewResolver(runtimegraph.Options{
			EnableCodeOwners:  cfg.RuntimeGraph.EnableCodeOwners,
			GraphManifestPath: cfg.RuntimeGraph.GraphManifestPath,
			MaxTraversalDepth: cfg.RuntimeGraph.MaxTraversalDepth,
		}),
		repository,
		controlplane.NewServiceWithOptions(
			repository,
			controlplane.WithLaneAssigner(lanes.NewAssigner(lanes.Config{
				GreenMax: cfg.Lanes.GreenMax, YellowMax: cfg.Lanes.YellowMax, RedMax: cfg.Lanes.RedMax,
			})),
			controlplane.WithCheckPublisher(checkPublisher),
		),
	)

	mux := http.NewServeMux()
	mux.Handle("/webhooks/github", ingest.NewWebhookHandler(processor, github.NewWebhookDecoder(cfg.GitHub.WebhookSecret)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &App{
		server: &http.Server{
			Addr:    cfg.Service.IngestAddress,
			Handler: mux,
		},
	}
}

func (a *App) Run() error {
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}
