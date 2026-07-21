package domain

// ConflictRoute describes the safe next action for a change that may no
// longer apply cleanly to its target branch. Merger deliberately only routes
// conflicts; it never attempts to resolve or rewrite a change.
type ConflictRoute string

const (
	ConflictRouteNone             ConflictRoute = "none"
	ConflictRouteRefreshAndVerify ConflictRoute = "refresh_and_verify"
	ConflictRouteHumanResolution  ConflictRoute = "human_resolution"
)

// ConflictFinding is an explainable signal used to route a potentially
// conflicting change. Paths refer only to files present in the analyzed diff.
type ConflictFinding struct {
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Paths    []string `json:"paths,omitempty"`
}

// ConflictAssessment records deterministic conflict and base-drift signals
// found while analyzing a Change Packet. It is guidance, not an auto-resolver.
type ConflictAssessment struct {
	BaseSHA                 string            `json:"baseSha,omitempty"`
	CurrentBaseSHA          string            `json:"currentBaseSha,omitempty"`
	Route                   ConflictRoute     `json:"route,omitempty"`
	Score                   int               `json:"score,omitempty"`
	RequiresHumanResolution bool              `json:"requiresHumanResolution,omitempty"`
	Findings                []ConflictFinding `json:"findings,omitempty"`
	Mitigations             []string          `json:"mitigations,omitempty"`
}
