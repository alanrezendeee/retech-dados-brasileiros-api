package domain

// Planos comerciais (Pricing v2)
const (
	PlanFree       = "free"
	PlanStarter    = "starter"
	PlanPro        = "pro"
	PlanBusiness   = "business"
	PlanEnterprise = "enterprise"
)

// IsValidPlan verifica se o plano é um dos planos conhecidos
func IsValidPlan(plan string) bool {
	switch plan {
	case PlanFree, PlanStarter, PlanPro, PlanBusiness, PlanEnterprise:
		return true
	}
	return false
}

// PlanLimits retorna os limites de rate limiting por plano.
// Enterprise é custom (configurado via tenant.RateLimit pelo admin);
// na ausência de configuração custom, usa os limites de Business como fallback.
func PlanLimits(plan string) RateLimitConfig {
	switch plan {
	case PlanStarter:
		return RateLimitConfig{RequestsPerDay: 1000, RequestsPerMinute: 30}
	case PlanPro:
		return RateLimitConfig{RequestsPerDay: 10000, RequestsPerMinute: 120}
	case PlanBusiness, PlanEnterprise:
		return RateLimitConfig{RequestsPerDay: 100000, RequestsPerMinute: 600}
	default: // free (e planos desconhecidos)
		return RateLimitConfig{RequestsPerDay: 100, RequestsPerMinute: 5}
	}
}

// PlanAllowsReverseCEP indica se o plano tem acesso à busca reversa de CEP (/cep/buscar).
// Disponível a partir do Starter. Tenants legados (sem plano gravado) são
// grandfathered e mantêm acesso.
func PlanAllowsReverseCEP(plan string) bool {
	return plan != PlanFree
}
