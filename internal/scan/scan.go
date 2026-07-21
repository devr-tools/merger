// Package scan runs the merger analysis pipeline offline against a raw unified
// diff, without the webhook ingest, event bus, GitHub App, or persistence
// dependencies of the control-plane services. It reuses the same core engines
// (mutations, runtime graph, risk, policy, lanes) so a local `merger scan`
// produces the same Change Packet and merge-lane decision the services would.
package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devr-tools/merger/internal/conflictrisk"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/risk"
	"github.com/devr-tools/merger/internal/runtimegraph"
	"github.com/devr-tools/merger/pkg/diff"
	"github.com/devr-tools/merger/pkg/identity"
)

// Options configures an offline scan.
type Options struct {
	// Diff is a raw unified diff (as produced by `git diff`).
	Diff string
	// RepoRoot is the filesystem root used to load file content for analyzers
	// that inspect file bodies (for example the OpenAPI/proto analyzer). It is
	// best-effort: missing files are treated as empty content.
	RepoRoot string
	// Repo optionally identifies the repository for the Change Packet.
	Repo domain.RepoRef
	// Ref is the revision the diff targets (used as the content-loader ref).
	Ref string
	// BaseSHA and CurrentBaseSHA must be supplied together to avoid inferring
	// target-branch drift from an ambiguous ref.
	BaseSHA        string
	CurrentBaseSHA string
	// Title optionally labels the Change Packet.
	Title string
	// Author optionally attributes the Change Packet.
	Author domain.Author
	// Policy is the policy rule set to evaluate. A zero value evaluates no
	// policies (every packet is approved by default).
	Policy policy.Config
	// Lanes configures the merge-lane thresholds.
	Lanes lanes.Config
	// EnableCodeOwners toggles CODEOWNERS-based ownership resolution.
	EnableCodeOwners bool
}

// fsContentLoader loads file content from the local filesystem. It satisfies
// both mutations.ContentLoader and runtimegraph.ContentLoader.
type fsContentLoader struct {
	root string
}

func (l fsContentLoader) Load(_ context.Context, path string) ([]byte, error) {
	if l.root == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(l.root, path))
	if err != nil {
		// Content is best-effort; analyzers tolerate empty content, and a file
		// present in a diff may be absent locally (deletions, foreign checkout).
		return nil, nil
	}
	return data, nil
}

// Run executes the full offline pipeline and returns the resulting Change
// Packet with mutations, runtime impact, risk, policy decision, and merge lane
// populated.
func Run(ctx context.Context, opts Options) (*domain.ChangePacket, error) {
	parsed, err := diff.ParseUnified(opts.Diff)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	now := time.Now().UTC()
	packet := &domain.ChangePacket{
		ID:        identity.New("cp"),
		Repo:      opts.Repo,
		PR:        domain.PullRequestRef{HeadSHA: opts.Ref, BaseSHA: opts.BaseSHA},
		Author:    opts.Author,
		Title:     opts.Title,
		Source:    "cli.scan",
		Files:     mapChangedFiles(parsed),
		MergeLane: domain.MergeLaneYellow,
		Decision:  domain.PolicyDecision{Status: domain.DecisionPending},
		Deployment: domain.DeploymentRequirement{
			Strategy: domain.DeployDirect,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if opts.CurrentBaseSHA != "" {
		packet.Metadata = map[string]string{"current_base_sha": opts.CurrentBaseSHA}
	}

	loader := fsContentLoader{root: opts.RepoRoot}

	mutationList, err := mutations.DefaultEngine().Classify(ctx, mutations.AnalysisRequest{
		Repo:    packet.Repo,
		Ref:     opts.Ref,
		Files:   packet.Files,
		Content: loader,
	})
	if err != nil {
		return nil, fmt.Errorf("classify mutations: %w", err)
	}
	packet.Mutations = mutationList
	packet.Conflict = conflictrisk.Analyze(*packet)

	runtimeImpact, ownership, err := runtimegraph.NewResolver(runtimegraph.Options{
		EnableCodeOwners: opts.EnableCodeOwners,
	}).ResolveImpact(ctx, runtimegraph.ResolutionInput{
		Packet: *packet,
		Ref:    opts.Ref,
		Loader: loader,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve runtime impact: %w", err)
	}
	packet.Runtime = runtimeImpact
	packet.Ownership = ownership

	riskSummary, risks, err := risk.DefaultEngine().Evaluate(ctx, *packet)
	if err != nil {
		return nil, fmt.Errorf("evaluate risk: %w", err)
	}
	packet.RiskSummary = riskSummary
	packet.Risks = risks

	evaluation, err := policy.NewRuleEngine(opts.Policy).Evaluate(ctx, *packet)
	if err != nil {
		return nil, fmt.Errorf("evaluate policy: %w", err)
	}
	packet.Decision = evaluation.Decision
	packet.Evidence = evaluation.Evidence
	packet.Reviewers = evaluation.Reviewers
	packet.Deployment = evaluation.Deployment

	mergeLane, err := lanes.NewAssigner(opts.Lanes).Assign(ctx, *packet)
	if err != nil {
		return nil, fmt.Errorf("assign merge lane: %w", err)
	}
	packet.MergeLane = mergeLane
	packet.UpdatedAt = time.Now().UTC()

	return packet, nil
}

// mapChangedFiles mirrors the ingest service's diff-to-domain mapping so
// offline scans classify files identically to webhook-driven ingestion.
func mapChangedFiles(files []diff.File) []domain.ChangedFile {
	mapped := make([]domain.ChangedFile, 0, len(files))
	for _, file := range files {
		mapped = append(mapped, domain.ChangedFile{
			Path:         file.Path,
			PreviousPath: file.PreviousPath,
			Status:       domain.FileStatus(file.Status),
			Language:     languageFromPath(file.Path),
			Additions:    file.Additions,
			Deletions:    file.Deletions,
			Changes:      file.Additions + file.Deletions,
			Patch:        file.Patch,
		})
	}
	return mapped
}

func languageFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".sql":
		return "sql"
	case ".yaml", ".yml":
		return "yaml"
	case ".proto":
		return "proto"
	default:
		return "unknown"
	}
}
