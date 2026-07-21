package domain

import "time"

type RepoRef struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	DefaultBranch string `json:"defaultBranch,omitempty"`
}

type PullRequestRef struct {
	Number  int    `json:"number"`
	URL     string `json:"url,omitempty"`
	HeadSHA string `json:"headSha,omitempty"`
	BaseSHA string `json:"baseSha,omitempty"`
}

type RiskSummary struct {
	Score        int        `json:"score"`
	Severity     Severity   `json:"severity"`
	Contributors []RiskType `json:"contributors,omitempty"`
}

type ChangePacket struct {
	ID          string                `json:"id"`
	Repo        RepoRef               `json:"repo"`
	PR          PullRequestRef        `json:"pullRequest"`
	Author      Author                `json:"author"`
	Title       string                `json:"title"`
	Summary     string                `json:"summary,omitempty"`
	Source      string                `json:"source,omitempty"`
	Files       []ChangedFile         `json:"files,omitempty"`
	Mutations   []Mutation            `json:"mutations,omitempty"`
	Risks       []Risk                `json:"risks,omitempty"`
	RiskSummary RiskSummary           `json:"riskSummary"`
	Conflict    ConflictAssessment    `json:"conflict,omitempty"`
	Evidence    []EvidenceRequirement `json:"evidence,omitempty"`
	Runtime     RuntimeImpact         `json:"runtimeImpact"`
	Ownership   []OwnershipBoundary   `json:"ownership,omitempty"`
	Reviewers   []ReviewerRequirement `json:"reviewers,omitempty"`
	Deployment  DeploymentRequirement `json:"deployment"`
	MergeLane   MergeLane             `json:"mergeLane"`
	Decision    PolicyDecision        `json:"decision"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
	CreatedAt   time.Time             `json:"createdAt"`
	UpdatedAt   time.Time             `json:"updatedAt"`
}
