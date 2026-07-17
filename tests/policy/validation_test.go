package policy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/policy"
)

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := writePolicy(t, `
policies:
  - name: auth
    when:
      mutations: [auth_behavior_change]
      mutatoins: [database_schema_mutation]
    action:
      minimum_lane: RED
`)

	_, err := policy.LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "mutatoins") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestValidateRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name    string
		config  policy.Config
		wantErr string
	}{
		{
			name: "empty name",
			config: policy.Config{Policies: []policy.RuleConfig{{
				When:   mutationCondition(),
				Action: policy.ActionClause{MinimumLane: domain.MergeLaneRed},
			}}},
			wantErr: "name must not be empty",
		},
		{
			name: "duplicate name",
			config: policy.Config{Policies: []policy.RuleConfig{
				validRule("Deploy"),
				validRule("deploy"),
			}},
			wantErr: "duplicate policy name",
		},
		{
			name: "empty condition",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name:   "global",
				Action: policy.ActionClause{Block: true},
			}}},
			wantErr: "at least one when condition",
		},
		{
			name: "empty effect",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name: "noop",
				When: mutationCondition(),
			}}},
			wantErr: "at least one requirement or action",
		},
		{
			name: "unsupported mutation",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name:   "mutation",
				When:   policy.WhenClause{Mutations: []domain.MutationKind{"magic_mutation"}},
				Action: policy.ActionClause{MinimumLane: domain.MergeLaneRed},
			}}},
			wantErr: "unsupported mutation kind",
		},
		{
			name: "empty evidence name",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name:    "evidence",
				When:    mutationCondition(),
				Require: policy.RequirementClause{Evidence: []string{" "}},
			}}},
			wantErr: "evidence name",
		},
		{
			name: "unsupported strategy",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name:    "deployment",
				When:    mutationCondition(),
				Require: policy.RequirementClause{Deployment: policy.DeploymentClause{Strategy: "instant"}},
			}}},
			wantErr: "unsupported deployment strategy",
		},
		{
			name: "unsupported minimum lane",
			config: policy.Config{Policies: []policy.RuleConfig{{
				Name:   "lane",
				When:   mutationCondition(),
				Action: policy.ActionClause{MinimumLane: "PURPLE"},
			}}},
			wantErr: "unsupported minimum lane",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := policy.Validate(test.config)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestValidateAllowsExtensibleEvidenceNames(t *testing.T) {
	rule := validRule("custom_evidence")
	rule.Require = policy.RequirementClause{Evidence: []string{"load_test_report"}}
	if err := policy.Validate(policy.Config{Policies: []policy.RuleConfig{rule}}); err != nil {
		t.Fatalf("expected non-empty custom evidence name to be valid, got %v", err)
	}
}

func validRule(name string) policy.RuleConfig {
	return policy.RuleConfig{
		Name:   name,
		When:   mutationCondition(),
		Action: policy.ActionClause{MinimumLane: domain.MergeLaneRed},
	}
}

func mutationCondition() policy.WhenClause {
	return policy.WhenClause{Mutations: []domain.MutationKind{domain.MutationAuthBehaviorChange}}
}

func writePolicy(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}
