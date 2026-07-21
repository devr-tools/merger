package runtimegraph

import (
	"context"
	"github.com/devr-tools/merger/internal/domain"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"strings"
)

type graphManifestSource struct{}

func (graphManifestSource) Name() string { return "graph-manifest-source" }

type graphManifest struct {
	Nodes []graphManifestNode `yaml:"nodes"`
	Edges []Edge              `yaml:"edges"`
}
type graphManifestNode struct {
	ID          string             `yaml:"id"`
	Name        string             `yaml:"name"`
	Kind        NodeKind           `yaml:"kind"`
	Owner       string             `yaml:"owner"`
	Criticality domain.Criticality `yaml:"criticality"`
	Paths       []string           `yaml:"paths"`
}

func (graphManifestSource) Collect(ctx context.Context, input ResolutionInput) (Fragment, error) {
	if input.Loader == nil || input.Options.GraphManifestPath == "" {
		return Fragment{}, nil
	}
	content, err := input.Loader.Load(ctx, input.Options.GraphManifestPath)
	if err != nil || len(content) == 0 {
		return Fragment{}, nil
	}
	var manifest graphManifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return Fragment{}, err
	}
	fragment := Fragment{Edges: manifest.Edges}
	for _, node := range manifest.Nodes {
		if node.ID == "" || node.Name == "" {
			continue
		}
		fragment.Nodes = append(fragment.Nodes, Node{ID: node.ID, Name: node.Name, Kind: node.Kind, Owner: node.Owner, Criticality: node.Criticality})
		fragment.Systems = append(fragment.Systems, domain.SystemRef{Kind: domain.SystemService, Name: node.Name, Owner: node.Owner, Criticality: node.Criticality})
		for _, file := range input.Packet.Files {
			if matchesGraphPath(node.Paths, file.Path) {
				fragment.Affected = append(fragment.Affected, node.ID)
				break
			}
		}
	}
	if len(fragment.Affected) > 0 {
		fragment.Notes = append(fragment.Notes, "graph manifest resolved transitive runtime dependencies")
	}
	return fragment, nil
}
func matchesGraphPath(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, path); ok || strings.HasPrefix(path, strings.TrimSuffix(pattern, "/**")) {
			return true
		}
	}
	return false
}

func traverseGraph(fragment Fragment, maxDepth int) Fragment {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	nodes := make(map[string]domain.SystemRef)
	for _, node := range fragment.Nodes {
		nodes[node.ID] = domain.SystemRef{Kind: domain.SystemService, Name: node.Name, Owner: node.Owner, Criticality: node.Criticality}
	}
	adjacency := make(map[string][]string)
	for _, edge := range fragment.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		adjacency[edge.To] = append(adjacency[edge.To], edge.From)
	}
	visited := make(map[string]bool)
	queue := append([]string(nil), fragment.Affected...)
	depth := make(map[string]int)
	for _, id := range queue {
		visited[id] = true
	}
	result := Fragment{Notes: fragment.Notes}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if system, ok := nodes[id]; ok {
			result.Systems = append(result.Systems, system)
			if criticalityRank(system.Criticality) > criticalityRank(result.Criticality) {
				result.Criticality = system.Criticality
			}
		}
		if depth[id] >= maxDepth {
			continue
		}
		for _, next := range adjacency[id] {
			if !visited[next] {
				visited[next] = true
				depth[next] = depth[id] + 1
				queue = append(queue, next)
			}
		}
	}
	return result
}
