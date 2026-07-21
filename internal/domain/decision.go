package domain

import "time"

type EvidenceType string

const (
	EvidenceUnitTests        EvidenceType = "unit_tests"
	EvidenceIntegrationTests EvidenceType = "integration_tests"
	EvidenceAuthTests        EvidenceType = "auth_integration_tests"
	EvidenceMigrationPlan    EvidenceType = "migration_plan"
	EvidenceRollbackPlan     EvidenceType = "rollback_plan"
	EvidenceSecurityReview   EvidenceType = "security_review"
	EvidenceRuntimeCanary    EvidenceType = "runtime_canary"
	EvidenceContractTests    EvidenceType = "contract_tests"
)

type EvidenceRequirement struct {
	Type        EvidenceType        `json:"type"`
	Name        string              `json:"name"`
	Required    bool                `json:"required"`
	Reason      string              `json:"reason,omitempty"`
	Producer    string              `json:"producer,omitempty"`
	GitHubCheck *GitHubCheckBinding `json:"githubCheck,omitempty"`
}

// GitHubCheckBinding identifies the only GitHub check that may automatically
// satisfy an evidence requirement. Both fields are required so a check with
// the same display name from another GitHub App cannot satisfy policy.
type GitHubCheckBinding struct {
	Name  string `json:"name"`
	AppID int64  `json:"appId"`
}

type MergeLane string

const (
	MergeLaneGreen  MergeLane = "GREEN"
	MergeLaneYellow MergeLane = "YELLOW"
	MergeLaneRed    MergeLane = "RED"
	MergeLaneBlack  MergeLane = "BLACK"
)

type DecisionStatus string

const (
	DecisionPending   DecisionStatus = "pending"
	DecisionApproved  DecisionStatus = "approved"
	DecisionEscalated DecisionStatus = "escalated"
	DecisionBlocked   DecisionStatus = "blocked"
)

type PolicyViolation struct {
	Policy   string   `json:"policy"`
	Reason   string   `json:"reason"`
	Severity Severity `json:"severity"`
}

type PolicyDecision struct {
	Status          DecisionStatus    `json:"status"`
	Summary         string            `json:"summary,omitempty"`
	Reasons         []string          `json:"reasons,omitempty"`
	Violations      []PolicyViolation `json:"violations,omitempty"`
	AppliedPolicies []string          `json:"appliedPolicies,omitempty"`
	MinimumLane     MergeLane         `json:"minimumLane,omitempty"`
	DecidedAt       time.Time         `json:"decidedAt,omitempty"`
}
