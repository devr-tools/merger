package risk

import "github.com/devr-tools/merger/internal/domain"

func defaultWeights() map[domain.MutationKind]int {
	return map[domain.MutationKind]int{
		domain.MutationAuthBehaviorChange:    35,
		domain.MutationDatabaseSchema:        32,
		domain.MutationRuntimeConfig:         20,
		domain.MutationAPIContract:           26,
		domain.MutationDependency:            15,
		domain.MutationInfrastructure:        28,
		domain.MutationDataAccess:            24,
		domain.MutationDeploymentWorkflow:    24,
		domain.MutationObservabilityContract: 12,
		domain.MutationUnknown:               8,
	}
}
