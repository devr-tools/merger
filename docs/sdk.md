# SDK Guide

The `github.com/devr-tools/merger/pkg/merger` package exposes merger's offline
analysis pipeline as a library. It runs the same mutation, runtime-graph, risk,
policy, and lane engines the control-plane services use, with no services,
database, or event bus required.

```bash
go get github.com/devr-tools/merger/pkg/merger
```

## Scan a diff

```go
import (
	"context"

	"github.com/devr-tools/merger/pkg/merger"
)

packet, err := merger.Scan(ctx, merger.ScanOptions{
	Diff:             rawUnifiedDiff,     // e.g. `git diff main...HEAD`
	RepoRoot:         ".",                // for analyzers that read file bodies
	Repo:             merger.RepoRef{Owner: "acme", Name: "payments", FullName: "acme/payments"},
	Ref:              "HEAD",
	Lanes:            merger.DefaultLanes(),
	EnableCodeOwners: true,
})
```

`Scan` returns a `*merger.ChangePacket` with `Mutations`, `Runtime`,
`RiskSummary`, `Decision`, `Reviewers`, `Evidence`, `Deployment`, and
`MergeLane` populated. The struct is JSON-serializable with the same shape the
services persist and publish.

## Apply policy

```go
policyConfig, err := merger.LoadPolicy(".merger/policies/default.yaml")
if err != nil {
	return err
}

packet, err := merger.Scan(ctx, merger.ScanOptions{
	Diff:   rawUnifiedDiff,
	Lanes:  merger.DefaultLanes(),
	Policy: policyConfig,
})
```

A zero `Policy` evaluates no rules (every packet is approved by default). A zero
`Lanes` disables thresholds; most callers want `merger.DefaultLanes()`.

## Types

`pkg/merger` re-exports the domain types (`ChangePacket`, `Mutation`,
`MergeLane`, `RiskSummary`, …) so callers never import `internal/` packages
directly.

## SCM adapters

`extensions.NewGitLabClient` and `extensions.NewBitbucketClient` are public
`SCMProvider` implementations for GitLab and Bitbucket Cloud. They implement
pull request retrieval, diff/content fetches, and provider-native commit status
publication using a token supplied by the embedding application. Configure
their API base URL, token (and Bitbucket username), then pass the adapter to
your integration; current service bootstrap remains GitHub-specific.

Policy construction is also supported through `merger.PolicyRule`,
`merger.PolicyRequirements`, and `merger.GitHubCheckBinding`. A GitHub check
binding pairs a declared evidence name with the exact check name and numeric
GitHub App ID permitted to satisfy it automatically.
