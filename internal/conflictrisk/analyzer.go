// Package conflictrisk detects merge-conflict and base-drift risk in a Change
// Packet. It intentionally provides routing guidance only: no source code is
// changed and no conflict is automatically resolved.
package conflictrisk

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
)

const currentBaseSHAMetadataKey = "current_base_sha"

// Analyze returns a deterministic assessment based solely on the packet's
// base/head diff data. A caller that knows the target branch's latest SHA may
// provide it through metadata[current_base_sha].
func Analyze(packet domain.ChangePacket) domain.ConflictAssessment {
	assessment := domain.ConflictAssessment{
		BaseSHA:        packet.PR.BaseSHA,
		CurrentBaseSHA: strings.TrimSpace(packet.Metadata[currentBaseSHAMetadataKey]),
		Route:          domain.ConflictRouteNone,
	}

	seen := make(map[string][]string)
	sensitive := make([]string, 0)
	markerPaths := make([]string, 0)
	for _, file := range packet.Files {
		path := normalizedPath(file.Path)
		if path == "" {
			continue
		}
		seen[path] = append(seen[path], file.Path)
		if hasConflictMarkers(file.Patch) {
			markerPaths = append(markerPaths, file.Path)
		}
		if isPolicySensitive(path) {
			sensitive = append(sensitive, file.Path)
		}
	}

	if assessment.BaseSHA != "" && assessment.CurrentBaseSHA != "" && assessment.BaseSHA != assessment.CurrentBaseSHA {
		assessment.Findings = append(assessment.Findings, domain.ConflictFinding{
			Kind: "base_drift", Severity: domain.SeverityHigh,
			Summary: "the target branch advanced after this change was based",
		})
		assessment.Score += 25
		assessment.Mitigations = append(assessment.Mitigations, "rebase or update the change onto the current target branch and rerun required checks")
		assessment.Route = domain.ConflictRouteRefreshAndVerify
	}

	for path, originalPaths := range seen {
		if len(originalPaths) < 2 {
			continue
		}
		assessment.Findings = append(assessment.Findings, domain.ConflictFinding{
			Kind: "overlapping_diff_entries", Severity: domain.SeverityMedium,
			Summary: "multiple diff entries modify the same normalized path", Paths: uniqueSorted(originalPaths),
		})
		assessment.Score += 20
		assessment.Mitigations = append(assessment.Mitigations, "inspect overlapping diff entries before merge and verify the resulting file content")
		if assessment.Route == domain.ConflictRouteNone {
			assessment.Route = domain.ConflictRouteRefreshAndVerify
		}
		_ = path
	}

	if len(sensitive) > 0 {
		assessment.Findings = append(assessment.Findings, domain.ConflictFinding{
			Kind: "policy_sensitive_files", Severity: domain.SeverityMedium,
			Summary: "the change touches files whose concurrent edits can alter policy, deployment, or ownership behavior", Paths: uniqueSorted(sensitive),
		})
		assessment.Score += 10
		assessment.Mitigations = append(assessment.Mitigations, "confirm policy-sensitive changes against the current target branch and obtain the designated owner review")
		if assessment.Route == domain.ConflictRouteNone {
			assessment.Route = domain.ConflictRouteRefreshAndVerify
		}
	}

	if len(markerPaths) > 0 {
		assessment.Findings = append(assessment.Findings, domain.ConflictFinding{
			Kind: "conflict_markers", Severity: domain.SeverityCritical,
			Summary: "unresolved merge-conflict markers are present in the proposed patch", Paths: uniqueSorted(markerPaths),
		})
		assessment.Score += 45
		assessment.RequiresHumanResolution = true
		assessment.Route = domain.ConflictRouteHumanResolution
		assessment.Mitigations = append(assessment.Mitigations, "resolve conflict markers manually, inspect the resulting diff, and rerun all required verification")
	}

	assessment.Score = min(assessment.Score, 100)
	return assessment
}

func hasConflictMarkers(patch string) bool {
	return strings.Contains(patch, "<<<<<<< ") && strings.Contains(patch, "=======") && strings.Contains(patch, ">>>>>>> ")
}

func normalizedPath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
}

func isPolicySensitive(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	lower := strings.ToLower(path)
	return base == "codeowners" || base == "merger.yaml" || strings.HasPrefix(lower, ".github/workflows/") ||
		strings.HasPrefix(lower, "config/") || strings.HasPrefix(lower, "deployments/") ||
		strings.Contains(lower, "terraform") || strings.Contains(lower, "migration") || strings.Contains(lower, "schema")
}

func uniqueSorted(paths []string) []string {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		set[path] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
