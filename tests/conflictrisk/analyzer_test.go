package conflictrisk_test

import (
	"testing"

	"github.com/devr-tools/merger/internal/conflictrisk"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/pkg/merger"
)

func TestAnalyzeRoutesConflictMarkersForHumanResolution(t *testing.T) {
	assessment := conflictrisk.Analyze(domain.ChangePacket{Files: []domain.ChangedFile{{
		Path:  "internal/auth.go",
		Patch: "+<<<<<<< HEAD\n+left\n+=======\n+right\n+>>>>>>> feature",
	}}})

	if assessment.Route != domain.ConflictRouteHumanResolution || !assessment.RequiresHumanResolution {
		t.Fatalf("expected human-resolution route, got %#v", assessment)
	}
	if assessment.Score != 45 || len(assessment.Findings) != 1 || assessment.Findings[0].Kind != "conflict_markers" {
		t.Fatalf("expected explainable marker finding, got %#v", assessment)
	}
}

func TestAnalyzeRoutesBaseDriftAndPolicySensitiveFiles(t *testing.T) {
	assessment := conflictrisk.Analyze(domain.ChangePacket{
		PR:       domain.PullRequestRef{BaseSHA: "base-at-open"},
		Metadata: map[string]string{"current_base_sha": "base-now"},
		Files:    []domain.ChangedFile{{Path: ".github/workflows/release.yml"}},
	})

	if assessment.Route != domain.ConflictRouteRefreshAndVerify {
		t.Fatalf("expected refresh route, got %#v", assessment)
	}
	if assessment.Score != 35 || len(assessment.Findings) != 2 {
		t.Fatalf("expected drift and sensitive-file findings, got %#v", assessment)
	}
}

func TestAnalyzeDoesNotRouteOrdinaryChange(t *testing.T) {
	assessment := conflictrisk.Analyze(domain.ChangePacket{Files: []domain.ChangedFile{{Path: "internal/orders.go"}}})
	if assessment.Route != domain.ConflictRouteNone || assessment.Score != 0 || len(assessment.Findings) != 0 {
		t.Fatalf("expected no conflict risk, got %#v", assessment)
	}
}

func TestSDKOptionsExposeBaseDriftInputs(t *testing.T) {
	opts := merger.ScanOptions{BaseSHA: "base-at-open", CurrentBaseSHA: "base-now"}
	if opts.BaseSHA == "" || opts.CurrentBaseSHA == "" {
		t.Fatalf("expected SDK scan options to expose base drift inputs")
	}
}
