package merger

import (
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/runtimegraph"
)

type Author = domain.Author
type ChangedFile = domain.ChangedFile
type ChangePacket = domain.ChangePacket
type RepoRef = domain.RepoRef
type PullRequestRef = domain.PullRequestRef
type Mutation = domain.Mutation
type MutationKind = domain.MutationKind
type MutationSignal = domain.MutationSignal
type Risk = domain.Risk
type RiskSummary = domain.RiskSummary
type ConflictAssessment = domain.ConflictAssessment
type ConflictFinding = domain.ConflictFinding
type ConflictRoute = domain.ConflictRoute
type EvidenceRequirement = domain.EvidenceRequirement
type EvidenceGitHubCheckBinding = domain.GitHubCheckBinding
type MergeLane = domain.MergeLane
type RuntimeImpact = domain.RuntimeImpact
type OwnershipBoundary = domain.OwnershipBoundary
type PolicyDecision = domain.PolicyDecision
type ReviewerRequirement = domain.ReviewerRequirement
type DeploymentRequirement = domain.DeploymentRequirement
type Severity = domain.Severity
type Criticality = domain.Criticality
type SystemRef = domain.SystemRef
type Envelope = events.Envelope
type EventType = events.EventType
type GraphNode = runtimegraph.Node
type GraphEdge = runtimegraph.Edge

const (
	EventPROpened                 = events.EventPROpened
	EventChangePacketCreated      = events.EventChangePacketCreated
	EventMutationDetected         = events.EventMutationDetected
	EventRiskAssigned             = events.EventRiskAssigned
	EventMergeLaneAssigned        = events.EventMergeLaneAssigned
	EventPolicyViolationDetected  = events.EventPolicyViolationDetected
	ConflictRouteNone             = domain.ConflictRouteNone
	ConflictRouteRefreshAndVerify = domain.ConflictRouteRefreshAndVerify
	ConflictRouteHumanResolution  = domain.ConflictRouteHumanResolution
)
