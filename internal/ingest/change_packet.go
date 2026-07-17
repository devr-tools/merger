package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/telemetry"
	"github.com/devr-tools/merger/pkg/diff"
	"github.com/devr-tools/merger/pkg/identity"
)

func (p *Processor) buildChangePacket(
	ctx context.Context,
	payload github.PullRequestWebhookPayload,
	pr github.PullRequest,
	service github.Service,
	repoOwner, repoName string,
	prNumber int,
) (*domain.ChangePacket, error) {
	rawDiff, err := service.GetPullRequestDiff(ctx, repoOwner, repoName, prNumber)
	if err != nil {
		return nil, fmt.Errorf("get pull request diff for %s/%s#%d: %w", repoOwner, repoName, prNumber, err)
	}

	parsedFiles, err := diff.ParseUnified(rawDiff)
	if err != nil {
		return nil, err
	}

	packet := &domain.ChangePacket{
		ID: identity.New("cp"),
		Repo: domain.RepoRef{
			Owner:    repoOwner,
			Name:     repoName,
			FullName: payload.Repository.FullName,
		},
		PR: domain.PullRequestRef{
			Number:  pr.Number,
			URL:     pr.URL,
			HeadSHA: pr.HeadSHA,
			BaseSHA: pr.BaseSHA,
		},
		Author: domain.Author{
			Login: pr.Author,
			Type:  payload.PullRequest.User.Type,
		},
		Title:     pr.Title,
		Summary:   strings.TrimSpace(pr.Body),
		Source:    "github.pull_request",
		Files:     mapChangedFiles(parsedFiles),
		MergeLane: domain.MergeLaneYellow,
		Decision: domain.PolicyDecision{
			Status: domain.DecisionPending,
		},
		Deployment: domain.DeploymentRequirement{
			Strategy: domain.DeployDirect,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Metadata: map[string]string{
			"correlation_id":  telemetry.CorrelationID(ctx),
			"installation_id": fmt.Sprintf("%d", payload.Installation.ID),
		},
	}

	if err := p.bus.Publish(ctx, events.NewEnvelope(events.EventChangePacketCreated, "ingest", packet)); err != nil {
		return nil, err
	}

	return packet, nil
}
