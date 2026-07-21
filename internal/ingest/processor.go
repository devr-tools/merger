package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/devr-tools/merger/internal/checks"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/risk"
	"github.com/devr-tools/merger/internal/runtimegraph"
	"github.com/devr-tools/merger/internal/store"
	"github.com/devr-tools/merger/internal/telemetry"
)

type Processor struct {
	logger    *telemetry.Logger
	tracer    telemetry.Tracer
	bus       events.Bus
	github    github.Service
	mutations mutations.Engine
	risk      risk.Engine
	policy    policy.Engine
	assigner  lanes.Assigner
	checks    checks.Publisher
	runtime   runtimegraph.Resolver
	store     store.ChangePacketStore
	evidence  EvidenceUpdater
}

// EvidenceUpdater is the control-plane boundary used by webhook ingestion.
// Keeping this narrow ensures GitHub-originated evidence follows the same
// validation, transition, persistence, and check-publication path as API updates.
type EvidenceUpdater interface {
	UpdateEvidenceExecution(context.Context, domain.EvidenceExecution) (domain.EvidenceExecution, error)
}

func NewProcessor(
	logger *telemetry.Logger,
	tracer telemetry.Tracer,
	bus events.Bus,
	githubService github.Service,
	mutationEngine mutations.Engine,
	riskEngine risk.Engine,
	policyEngine policy.Engine,
	assigner lanes.Assigner,
	checkPublisher checks.Publisher,
	runtimeResolver runtimegraph.Resolver,
	packetStore store.ChangePacketStore,
	evidenceUpdaters ...EvidenceUpdater,
) *Processor {
	var evidenceUpdater EvidenceUpdater
	if len(evidenceUpdaters) > 0 {
		evidenceUpdater = evidenceUpdaters[0]
	}
	return &Processor{
		logger:    logger,
		tracer:    tracer,
		bus:       bus,
		github:    githubService,
		mutations: mutationEngine,
		risk:      riskEngine,
		policy:    policyEngine,
		assigner:  assigner,
		checks:    checkPublisher,
		runtime:   runtimeResolver,
		store:     packetStore,
		evidence:  evidenceUpdater,
	}
}

// ProcessCheckRun reconciles a GitHub check only when it is explicitly bound
// by policy to evidence on the exact repository, pull request, and head SHA.
func (p *Processor) ProcessCheckRun(ctx context.Context, payload github.CheckRunWebhookPayload) error {
	if p.evidence == nil || p.store == nil || payload.Action != "completed" {
		return nil
	}

	status, ok := evidenceStatusForCheckRun(payload.CheckRun.Status, payload.CheckRun.Conclusion)
	if !ok || payload.CheckRun.HeadSHA == "" || payload.Repository.FullName == "" {
		return nil
	}

	for _, pullRequest := range payload.CheckRun.PullRequests {
		if err := p.reconcileCheckRunForPullRequest(ctx, payload, pullRequest.Number, status); err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) reconcileCheckRunForPullRequest(ctx context.Context, payload github.CheckRunWebhookPayload, prNumber int, status domain.EvidenceStatus) error {
	packet, err := p.store.FindLatestChangePacket(ctx, payload.Repository.FullName, prNumber, payload.CheckRun.HeadSHA)
	if errors.Is(err, store.ErrChangePacketNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find change packet for GitHub check reconciliation: %w", err)
	}
	requirement, ok := matchingEvidenceRequirement(packet.Evidence, payload.CheckRun.Name, payload.CheckRun.App.ID)
	if !ok {
		return nil
	}
	summary := payload.CheckRun.Output.Summary
	if summary == "" {
		summary = payload.CheckRun.Output.Title
	}
	_, err = p.evidence.UpdateEvidenceExecution(ctx, domain.EvidenceExecution{
		ChangePacketID: packet.ID,
		Name:           requirement.Name,
		Status:         status,
		Summary:        summary,
		DetailsURL:     payload.CheckRun.DetailsURL,
		UpdatedBy:      fmt.Sprintf("github-app:%d", payload.CheckRun.App.ID),
		Metadata: map[string]string{
			"github_check_run_id": fmt.Sprintf("%d", payload.CheckRun.ID),
			"github_check_name":   payload.CheckRun.Name,
			"github_app_id":       fmt.Sprintf("%d", payload.CheckRun.App.ID),
			"github_head_sha":     payload.CheckRun.HeadSHA,
		},
	})
	if err != nil {
		return fmt.Errorf("reconcile GitHub check %q for change packet %s: %w", payload.CheckRun.Name, packet.ID, err)
	}
	return nil
}

func (p *Processor) ProcessPROpened(ctx context.Context, payload github.PullRequestWebhookPayload) (*domain.ChangePacket, error) {
	return p.processPROpened(ctx, payload)
}
