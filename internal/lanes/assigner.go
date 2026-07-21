package lanes

import (
	"context"

	"github.com/devr-tools/merger/internal/domain"
)

type Config struct {
	GreenMax  int
	YellowMax int
	RedMax    int
}

type Assigner interface {
	Assign(context.Context, domain.ChangePacket) (domain.MergeLane, error)
}

type ThresholdAssigner struct {
	config Config
}

func NewAssigner(config Config) *ThresholdAssigner {
	return &ThresholdAssigner{config: config}
}

func (a *ThresholdAssigner) Assign(_ context.Context, packet domain.ChangePacket) (domain.MergeLane, error) {
	if lane, overridden := forcedLane(packet); overridden {
		return lane, nil
	}

	if packet.RiskSummary.Score <= a.config.GreenMax && len(packet.Reviewers) == 0 && !packet.Deployment.RequiresCanary {
		return maxLane(domain.MergeLaneGreen, packet.Decision.MinimumLane), nil
	}

	if packet.RiskSummary.Score <= a.config.YellowMax && !requiresEscalatedReview(packet) {
		return maxLane(domain.MergeLaneYellow, packet.Decision.MinimumLane), nil
	}

	if packet.RiskSummary.Score <= a.config.RedMax || requiresEscalatedReview(packet) || packet.Deployment.RequiresCanary {
		return maxLane(domain.MergeLaneRed, packet.Decision.MinimumLane), nil
	}

	return domain.MergeLaneBlack, nil
}

func forcedLane(packet domain.ChangePacket) (domain.MergeLane, bool) {
	if packet.Decision.Status == domain.DecisionBlocked || packet.Conflict.RequiresHumanResolution {
		return domain.MergeLaneBlack, true
	}
	if packet.Conflict.Route == domain.ConflictRouteRefreshAndVerify {
		return maxLane(domain.MergeLaneRed, packet.Decision.MinimumLane), true
	}
	if packet.Decision.MinimumLane == domain.MergeLaneBlack {
		return domain.MergeLaneBlack, true
	}
	if packet.Decision.Status == domain.DecisionPending || packet.Decision.Status == domain.DecisionEscalated {
		return maxLane(domain.MergeLaneRed, packet.Decision.MinimumLane), true
	}
	return "", false
}

func requiresEscalatedReview(packet domain.ChangePacket) bool {
	for _, reviewer := range packet.Reviewers {
		if reviewer.Mandatory {
			return true
		}
	}
	return false
}

func maxLane(aLane, bLane domain.MergeLane) domain.MergeLane {
	if bLane == "" {
		return aLane
	}

	order := map[domain.MergeLane]int{
		domain.MergeLaneGreen:  1,
		domain.MergeLaneYellow: 2,
		domain.MergeLaneRed:    3,
		domain.MergeLaneBlack:  4,
	}

	if order[bLane] > order[aLane] {
		return bLane
	}
	return aLane
}
