package ingest

import (
	"github.com/devr-tools/merger/internal/domain"
)

func evidenceStatusForCheckRun(status, conclusion string) (domain.EvidenceStatus, bool) {
	if status != "completed" {
		return "", false
	}
	if conclusion == "success" {
		return domain.EvidenceSatisfied, true
	}
	// A completed check that did not explicitly succeed must never satisfy
	// required evidence. This includes neutral, skipped, cancelled, timed_out,
	// action_required, stale, and unknown GitHub conclusions.
	return domain.EvidenceFailed, true
}

func matchingEvidenceRequirement(requirements []domain.EvidenceRequirement, checkName string, appID int64) (domain.EvidenceRequirement, bool) {
	for _, requirement := range requirements {
		binding := requirement.GitHubCheck
		if binding != nil && binding.Name == checkName && binding.AppID == appID {
			return requirement, true
		}
	}
	return domain.EvidenceRequirement{}, false
}
