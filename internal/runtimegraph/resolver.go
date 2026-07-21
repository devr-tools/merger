package runtimegraph

import (
	"context"

	"github.com/devr-tools/merger/internal/domain"
)

type Builder struct {
	sources []Source
}

func NewResolver(options Options) *Builder {
	sources := []Source{
		repositoryTopologySource{},
		manifestSource{},
	}
	if options.GraphManifestPath != "" {
		sources = append(sources, graphManifestSource{})
	}
	if options.EnableCodeOwners {
		sources = append(sources, codeOwnersSource{})
	}
	return &Builder{sources: sources}
}

func (r *Builder) ResolveImpact(ctx context.Context, input ResolutionInput) (domain.RuntimeImpact, []domain.OwnershipBoundary, error) {
	serviceIndex := make(map[string]domain.SystemRef)
	ownerIndex := make(map[string]domain.OwnershipBoundary)
	impact := defaultImpact()

	for _, source := range r.sources {
		fragment, err := source.Collect(ctx, input)
		if err != nil {
			return domain.RuntimeImpact{}, nil, err
		}

		if len(fragment.Edges) > 0 && len(fragment.Affected) > 0 {
			resolved := traverseGraph(fragment, input.Options.MaxTraversalDepth)
			indexSystems(serviceIndex, resolved.Systems)
			promoteCriticality(&impact, resolved.Criticality)
			impact.Notes = append(impact.Notes, resolved.Notes...)
		} else {
			indexSystems(serviceIndex, fragment.Systems)
		}
		indexOwners(ownerIndex, fragment.Ownership)
		promoteCriticality(&impact, fragment.Criticality)
		impact.Notes = append(impact.Notes, fragment.Notes...)
	}

	impact.BlastRadius = deriveBlastRadius(serviceIndex)
	impact.Services = materializeSystems(serviceIndex)

	owners := materializeOwners(ownerIndex)
	if len(impact.Notes) == 0 {
		impact.Notes = []string{"runtime graph sources did not find additional topology metadata"}
	}

	return impact, owners, nil
}

func defaultImpact() domain.RuntimeImpact {
	return domain.RuntimeImpact{
		BlastRadius: domain.BlastRadiusUnknown,
		Criticality: domain.CriticalityNormal,
	}
}

func indexSystems(index map[string]domain.SystemRef, systems []domain.SystemRef) {
	for _, system := range systems {
		index[system.Name] = system
	}
}

func indexOwners(index map[string]domain.OwnershipBoundary, owners []domain.OwnershipBoundary) {
	for _, owner := range owners {
		index[owner.Team] = owner
	}
}

func promoteCriticality(impact *domain.RuntimeImpact, candidate domain.Criticality) {
	if criticalityRank(candidate) > criticalityRank(impact.Criticality) {
		impact.Criticality = candidate
	}
}

func deriveBlastRadius(services map[string]domain.SystemRef) domain.BlastRadius {
	switch len(services) {
	case 0:
		return domain.BlastRadiusUnknown
	case 1:
		return domain.BlastRadiusIsolated
	default:
		return domain.BlastRadiusLocalized
	}
}

func materializeSystems(index map[string]domain.SystemRef) []domain.SystemRef {
	systems := make([]domain.SystemRef, 0, len(index))
	for _, system := range index {
		systems = append(systems, system)
	}
	return systems
}

func materializeOwners(index map[string]domain.OwnershipBoundary) []domain.OwnershipBoundary {
	owners := make([]domain.OwnershipBoundary, 0, len(index))
	for _, owner := range index {
		owners = append(owners, owner)
	}
	return owners
}
