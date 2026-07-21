package merger

import (
	"context"

	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/scan"
)

// ScanOptions configures an offline scan. See Scan.
type ScanOptions = scan.Options

// PolicyConfig is a merger policy rule set.
type PolicyConfig = policy.Config
type PolicyRule = policy.RuleConfig
type PolicyRequirements = policy.RequirementClause
type GitHubCheckBinding = policy.GitHubCheckBinding

// LanesConfig configures merge-lane thresholds.
type LanesConfig = lanes.Config

// DefaultLanes returns merger's default merge-lane thresholds.
func DefaultLanes() LanesConfig {
	d := config.Defaults().Lanes
	return LanesConfig{GreenMax: d.GreenMax, YellowMax: d.YellowMax, RedMax: d.RedMax}
}

// LoadPolicy reads a policy rule set from a YAML file. A nil error with an empty
// rule set means the file parsed but declared no policies.
func LoadPolicy(path string) (PolicyConfig, error) {
	return policy.LoadConfig(path)
}

// Scan runs the merger analysis pipeline offline against opts.Diff — mutation
// detection, runtime-graph resolution, risk scoring, policy evaluation, and
// merge-lane assignment — and returns the resulting Change Packet. It requires
// no services, database, or event bus, so it is safe to call from a CLI, a CI
// job, or an agent tool.
//
// The zero value of opts.Lanes disables lane thresholds; most callers want
// DefaultLanes(). opts.Policy may be loaded with LoadPolicy or left empty to
// evaluate no policies.
func Scan(ctx context.Context, opts ScanOptions) (*ChangePacket, error) {
	return scan.Run(ctx, opts)
}
