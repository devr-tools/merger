package domain

type RiskType string

const (
	RiskRuntime    RiskType = "runtime"
	RiskSecurity   RiskType = "security"
	RiskSchema     RiskType = "schema"
	RiskDependency RiskType = "dependency"
	RiskRollout    RiskType = "rollout"
	RiskOwnership  RiskType = "ownership"
	RiskConflict   RiskType = "conflict"
)

type Risk struct {
	Type            RiskType `json:"type"`
	Score           int      `json:"score"`
	Severity        Severity `json:"severity"`
	Summary         string   `json:"summary"`
	Reason          string   `json:"reason,omitempty"`
	AffectedSystems []string `json:"affectedSystems,omitempty"`
	Mitigations     []string `json:"mitigations,omitempty"`
}
