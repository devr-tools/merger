package ingestapp

import (
	"context"
	"net/http"
	"time"

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
	externalAnalyzers := make([]mutations.Analyzer, 0, len(cfg.MutationAnalyzers))
	for _, declaration := range cfg.MutationAnalyzers {
		timeout := 5 * time.Second
		if declaration.Timeout != "" {
			if parsed, err := time.ParseDuration(declaration.Timeout); err == nil {
				timeout = parsed
			}
		}
		analyzer, err := mutations.NewExternalAnalyzer(mutations.ExternalAnalyzerConfig{Name: declaration.Name, Executable: declaration.Executable, Allowlist: declaration.Allowlist, Timeout: timeout, Paths: declaration.Paths})
		if err != nil {
			panic(err)
		}
		externalAnalyzers = append(externalAnalyzers, analyzer)
	}
	checkPublisher := checks.NewGitHubCheckPublisher(githubService)
	processor := ingest.NewProcessor(
		logger,
		tracer,
		bus,
		githubService,
		mutations.DefaultEngineWithExternal(externalAnalyzers),
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
