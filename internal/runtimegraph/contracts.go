package runtimegraph

import (
	"context"

	"github.com/devr-tools/merger/internal/domain"
)

type Snapshot interface {
	Nodes(context.Context) ([]Node, error)
	Edges(context.Context) ([]Edge, error)
}

type ContentLoader interface {
	Load(context.Context, string) ([]byte, error)
}

type ResolutionInput struct {
	Packet  domain.ChangePacket
	Ref     string
	Loader  ContentLoader
	Options Options
}

type Options struct {
	EnableCodeOwners  bool
	GraphManifestPath string
	MaxTraversalDepth int
}

type Resolver interface {
	ResolveImpact(context.Context, ResolutionInput) (domain.RuntimeImpact, []domain.OwnershipBoundary, error)
}

type Source interface {
	Name() string
	Collect(context.Context, ResolutionInput) (Fragment, error)
}

type Fragment struct {
	Nodes       []Node
	Edges       []Edge
	Systems     []domain.SystemRef
	Ownership   []domain.OwnershipBoundary
	Notes       []string
	Criticality domain.Criticality
	Affected    []string
}
