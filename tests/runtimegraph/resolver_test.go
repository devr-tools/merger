package runtimegraph_test

import (
	"context"
	"testing"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/runtimegraph"
)

func TestResolverDerivesOwnershipFromTopology(t *testing.T) {
	resolver := runtimegraph.NewResolver(runtimegraph.Options{})

	impact, owners, err := resolver.ResolveImpact(context.Background(), runtimegraph.ResolutionInput{
		Packet: domain.ChangePacket{
			Files: []domain.ChangedFile{
				{Path: "internal/auth/jwt.go"},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve impact: %v", err)
	}
	if impact.BlastRadius != domain.BlastRadiusIsolated {
		t.Fatalf("expected isolated blast radius, got %s", impact.BlastRadius)
	}
	if len(owners) == 0 || owners[0].Team != "auth" {
		t.Fatalf("expected auth owner, got %#v", owners)
	}
}

func TestResolverIncludesManifestDerivedService(t *testing.T) {
	resolver := runtimegraph.NewResolver(runtimegraph.Options{})
	loader := stubLoader{
		files: map[string][]byte{
			"deploy/api.yaml": []byte("kind: Deployment\nmetadata:\n  name: payments-api\n"),
		},
	}

	impact, owners, err := resolver.ResolveImpact(context.Background(), runtimegraph.ResolutionInput{
		Packet: domain.ChangePacket{
			Files: []domain.ChangedFile{
				{Path: "deploy/api.yaml"},
			},
		},
		Loader: loader,
	})
	if err != nil {
		t.Fatalf("resolve impact: %v", err)
	}
	if len(impact.Services) != 1 || impact.Services[0].Name != "payments-api" {
		t.Fatalf("expected manifest-derived service, got %#v", impact.Services)
	}
	if len(owners) != 0 {
		t.Fatalf("expected no owners from manifest-only change, got %#v", owners)
	}
}

func TestResolverMapsCodeOwnersWhenEnabled(t *testing.T) {
	resolver := runtimegraph.NewResolver(runtimegraph.Options{EnableCodeOwners: true})
	loader := stubLoader{
		files: map[string][]byte{
			".github/CODEOWNERS": []byte("/internal/auth/ @security\n"),
		},
	}

	_, owners, err := resolver.ResolveImpact(context.Background(), runtimegraph.ResolutionInput{
		Packet: domain.ChangePacket{
			Files: []domain.ChangedFile{
				{Path: "internal/auth/jwt.go"},
			},
		},
		Loader: loader,
	})
	if err != nil {
		t.Fatalf("resolve impact: %v", err)
	}
	if len(owners) != 2 {
		t.Fatalf("expected topology and codeowners boundaries, got %#v", owners)
	}
}

func TestResolverTraversesConfiguredGraphManifestWithinBound(t *testing.T) {
	resolver := runtimegraph.NewResolver(runtimegraph.Options{GraphManifestPath: ".merger/runtime-graph.yaml", MaxTraversalDepth: 1})
	loader := stubLoader{files: map[string][]byte{
		".merger/runtime-graph.yaml": []byte(`nodes:
  - id: api
    name: payments-api
    kind: service
    criticality: high
    paths: ["services/api/**"]
  - id: worker
    name: payments-worker
    kind: service
  - id: ledger
    name: ledger
    kind: service
edges:
  - from: api
    to: worker
    type: calls
  - from: worker
    to: ledger
    type: calls
`),
	}}
	impact, _, err := resolver.ResolveImpact(context.Background(), runtimegraph.ResolutionInput{Packet: domain.ChangePacket{Files: []domain.ChangedFile{{Path: "services/api/handler.go"}}}, Loader: loader, Options: runtimegraph.Options{GraphManifestPath: ".merger/runtime-graph.yaml", MaxTraversalDepth: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Services) != 3 || impact.Criticality != domain.CriticalityHigh {
		t.Fatalf("expected bounded api/worker impact, got %#v", impact)
	}
	for _, service := range impact.Services {
		if service.Name == "ledger" {
			t.Fatalf("depth-limited traversal must not include ledger: %#v", impact.Services)
		}
	}
}
