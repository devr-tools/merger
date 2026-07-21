package risk

import (
	"context"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
)

func (e *WeightedEngine) Evaluate(_ context.Context, packet domain.ChangePacket) (domain.RiskSummary, []domain.Risk, error) {
	riskByType := make(map[domain.RiskType]*domain.Risk)
	score := 0

	for _, mutation := range packet.Mutations {
		score += e.applyMutationRisk(riskByType, mutation)
	}
	if packet.Conflict.Score > 0 {
		conflict := &domain.Risk{
			Type: domain.RiskConflict, Score: packet.Conflict.Score,
			Severity:    severityFromScore(packet.Conflict.Score),
			Summary:     "merge conflict or target-branch drift risk detected",
			Mitigations: packet.Conflict.Mitigations,
		}
		for _, finding := range packet.Conflict.Findings {
			conflict.Reason = strings.TrimSpace(conflict.Reason + "; " + finding.Kind)
			conflict.AffectedSystems = appendUnique(conflict.AffectedSystems, finding.Paths...)
		}
		riskByType[domain.RiskConflict] = conflict
		score += conflict.Score
	}

	score = applyRuntimeCriticality(score, packet.Runtime.Criticality)
	score = applyChangeVolume(score, len(packet.Files))
	score = clampScore(score)

	risks, contributors := materializeRisks(riskByType)
	return domain.RiskSummary{
		Score:        score,
		Severity:     severityFromScore(score),
		Contributors: contributors,
	}, risks, nil
}

func (e *WeightedEngine) applyMutationRisk(riskByType map[domain.RiskType]*domain.Risk, mutation domain.Mutation) int {
	base := e.weights[mutation.Kind]
	riskType, summary, mitigations := classifyRisk(mutation.Kind)

	current := riskByType[riskType]
	if current == nil {
		current = &domain.Risk{
			Type:        riskType,
			Summary:     summary,
			Mitigations: mitigations,
			Severity:    mutation.Severity,
		}
		riskByType[riskType] = current
	}

	current.Score += base
	current.Reason = strings.TrimSpace(current.Reason + "; mutation=" + string(mutation.Kind))
	current.AffectedSystems = appendUnique(current.AffectedSystems, mutation.Files...)
	return base
}

func applyRuntimeCriticality(score int, criticality domain.Criticality) int {
	switch criticality {
	case domain.CriticalityHigh:
		return score + 10
	case domain.CriticalityTier0:
		return score + 20
	default:
		return score
	}
}

func applyChangeVolume(score int, fileCount int) int {
	if fileCount > 20 {
		return score + 8
	}
	return score
}

func materializeRisks(riskByType map[domain.RiskType]*domain.Risk) ([]domain.Risk, []domain.RiskType) {
	risks := make([]domain.Risk, 0, len(riskByType))
	contributors := make([]domain.RiskType, 0, len(riskByType))

	for _, risk := range riskByType {
		risk.Severity = severityFromScore(risk.Score)
		risks = append(risks, *risk)
		contributors = append(contributors, risk.Type)
	}

	return risks, contributors
}
