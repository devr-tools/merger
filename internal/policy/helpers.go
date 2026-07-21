package policy

import (
	"strings"

	"github.com/devr-tools/merger/internal/domain"
)

func appendUniqueReviewer(values []domain.ReviewerRequirement, candidate domain.ReviewerRequirement) []domain.ReviewerRequirement {
	for _, value := range values {
		if strings.EqualFold(value.Team, candidate.Team) {
			return values
		}
	}
	return append(values, candidate)
}

func appendUniqueEvidence(values []domain.EvidenceRequirement, candidate domain.EvidenceRequirement) []domain.EvidenceRequirement {
	for index, value := range values {
		if value.Name == candidate.Name {
			// A binding is more specific than legacy scalar evidence. Retain it
			// independently of matched-policy order, while validation rejects two
			// conflicting bindings for the same evidence name.
			if value.GitHubCheck == nil && candidate.GitHubCheck != nil {
				values[index].GitHubCheck = candidate.GitHubCheck
			}
			return values
		}
	}
	return append(values, candidate)
}

func maxLane(a, b domain.MergeLane) domain.MergeLane {
	if a == "" {
		return b
	}

	order := map[domain.MergeLane]int{
		domain.MergeLaneGreen:  1,
		domain.MergeLaneYellow: 2,
		domain.MergeLaneRed:    3,
		domain.MergeLaneBlack:  4,
	}
	if order[b] > order[a] {
		return b
	}
	return a
}
