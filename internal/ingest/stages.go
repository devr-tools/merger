package ingest

import (
	"context"
	"time"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/runtimegraph"
)

func (p *Processor) enrichMutations(ctx context.Context, packet *domain.ChangePacket, service github.Service, repoOwner, repoName string) error {
	loader := newGitHubContentLoader(service, repoOwner, repoName, packet.PR.HeadSHA)

	mutationList, err := p.mutations.Classify(ctx, mutations.AnalysisRequest{
		Repo:    packet.Repo,
		Ref:     packet.PR.HeadSHA,
		Files:   packet.Files,
		Content: loader,
	})
	if err != nil {
		return err
	}

	packet.Mutations = mutationList
	return p.publishMutations(ctx, packet.Mutations)
}

func (p *Processor) enrichRuntimeImpact(ctx context.Context, packet *domain.ChangePacket, service github.Service, repoOwner, repoName string) error {
	loader := newGitHubContentLoader(service, repoOwner, repoName, packet.PR.HeadSHA)

	runtimeImpact, ownership, err := p.runtime.ResolveImpact(ctx, runtimegraph.ResolutionInput{
		Packet: *packet,
		Ref:    packet.PR.HeadSHA,
		Loader: loader,
	})
	if err != nil {
		return err
	}

	packet.Runtime = runtimeImpact
	packet.Ownership = ownership
	return nil
}

func (p *Processor) enrichRisk(ctx context.Context, packet *domain.ChangePacket) error {
	riskSummary, risks, err := p.risk.Evaluate(ctx, *packet)
	if err != nil {
		return err
	}

	packet.RiskSummary = riskSummary
	packet.Risks = risks
	return p.publishRisk(ctx, packet.Risks)
}

func (p *Processor) applyPolicy(ctx context.Context, packet *domain.ChangePacket) error {
	policyEval, err := p.policy.Evaluate(ctx, *packet)
	if err != nil {
		return err
	}

	packet.Decision = policyEval.Decision
	packet.Evidence = policyEval.Evidence
	packet.Reviewers = policyEval.Reviewers
	packet.Deployment = policyEval.Deployment
	return nil
}

func (p *Processor) assignMergeLane(ctx context.Context, packet *domain.ChangePacket) error {
	mergeLane, err := p.assigner.Assign(ctx, *packet)
	if err != nil {
		return err
	}

	packet.MergeLane = mergeLane
	packet.UpdatedAt = time.Now().UTC()
	return nil
}

func (p *Processor) finalize(ctx context.Context, packet *domain.ChangePacket) error {
	if len(packet.Decision.Violations) > 0 {
		if err := p.publishViolations(ctx, packet.Decision.Violations); err != nil {
			return err
		}
	}

	if err := p.publishLane(ctx, packet.ID, packet.MergeLane); err != nil {
		return err
	}

	if p.store != nil {
		if err := p.store.SaveChangePacket(ctx, *packet); err != nil {
			return err
		}
	}
	// Persist before creating the GitHub check run. GitHub can deliver a
	// check_run webhook immediately after creation; storing first guarantees
	// reconciliation can bind that webhook to this exact packet.
	if err := p.checks.Publish(ctx, *packet); err != nil {
		return err
	}

	p.logger.Info("processed pull request",
		"change_packet_id", packet.ID,
		"repo", packet.Repo.FullName,
		"pr_number", packet.PR.Number,
		"lane", string(packet.MergeLane),
		"risk_score", packet.RiskSummary.Score,
	)

	return nil
}
