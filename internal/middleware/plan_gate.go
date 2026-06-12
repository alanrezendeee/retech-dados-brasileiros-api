package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theretech/retech-core/internal/domain"
	"github.com/theretech/retech-core/internal/storage"
)

// PlanGate aplica restrições de recursos por plano (Pricing v2)
type PlanGate struct {
	tenants *storage.TenantsRepo
}

func NewPlanGate(tenants *storage.TenantsRepo) *PlanGate {
	return &PlanGate{tenants: tenants}
}

// RequireReverseCEP bloqueia a busca reversa de CEP (/cep/buscar) para o plano free.
// Tenants legados sem plano gravado são grandfathered (acesso mantido).
func (pg *PlanGate) RequireReverseCEP() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantIDValue, exists := c.Get("tenant_id")
		if !exists {
			// Sem tenant no contexto (rota pública/demo) — não aplica gate de plano
			c.Next()
			return
		}

		tenant, err := pg.tenants.ByTenantID(c.Request.Context(), tenantIDValue.(string))
		if err != nil || tenant == nil {
			// Falha ao buscar tenant — graceful degradation, não bloqueia
			c.Next()
			return
		}

		if !domain.PlanAllowsReverseCEP(tenant.Plan) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "upgrade_required",
				"message": "Busca reversa de CEP disponível a partir do plano Starter",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
