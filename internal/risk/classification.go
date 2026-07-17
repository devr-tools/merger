package risk

import "github.com/devr-tools/merger/internal/domain"

func classifyRisk(kind domain.MutationKind) (domain.RiskType, string, []string) {
	switch kind {
	case domain.MutationAuthBehaviorChange:
		return domain.RiskSecurity, "security control surface changed", []string{"security review", "auth integration tests"}
	case domain.MutationDatabaseSchema:
		return domain.RiskSchema, "schema compatibility may affect runtime behavior", []string{"migration plan", "rollback validation"}
	case domain.MutationRuntimeConfig, domain.MutationDeploymentWorkflow:
		return domain.RiskRollout, "rollout behavior changed", []string{"canary", "runtime smoke tests"}
	case domain.MutationDependency:
		return domain.RiskDependency, "dependency graph changed", []string{"dependency diff review", "build provenance"}
	case domain.MutationDataAccess:
		return domain.RiskRuntime, "data access behavior changed", []string{"integration tests", "data consistency validation"}
	default:
		return domain.RiskRuntime, "runtime behavior may have shifted", []string{"integration tests"}
	}
}
