package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theretech/retech-core/internal/domain"
	"github.com/theretech/retech-core/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RateLimiter struct {
	db       *mongo.Database
	tenants  *storage.TenantsRepo
	settings *storage.SettingsRepo
}

func NewRateLimiter(db *mongo.Database, tenants *storage.TenantsRepo, settings *storage.SettingsRepo) *RateLimiter {
	return &RateLimiter{
		db:       db,
		tenants:  tenants,
		settings: settings,
	}
}

// Middleware aplica rate limiting baseado em API Key
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Println("🔄 [RATE LIMITER] Middleware chamado!")

		// Extrair API Key do contexto (já foi validada pelo middleware de API Key)
		apiKeyValue, exists := c.Get("api_key")
		if !exists {
			fmt.Println("⚠️  [RATE LIMITER] Nenhuma API key no contexto, passando...")
			// Se não tem API key, deixa passar (rota pública)
			c.Next()
			return
		}

		// Extrair tenant_id do contexto
		tenantIDValue, exists := c.Get("tenant_id")
		if !exists {
			fmt.Println("⚠️  [RATE LIMITER] Nenhum tenant_id no contexto, passando...")
			// Se não tem tenant_id, deixa passar
			c.Next()
			return
		}

		apiKey := apiKeyValue.(string)
		tenantID := tenantIDValue.(string)

		fmt.Printf("🔑 [RATE LIMITER] API Key: %s... | Tenant: %s\n", apiKey[:20], tenantID)

		// Buscar configuração de rate limit para o tenant
		config := rl.getRateLimitConfig(tenantID)

		fmt.Printf("🔍 Rate Limit Config para tenant %s: %d/dia, %d/min\n", tenantID, config.RequestsPerDay, config.RequestsPerMinute)

		ctx := context.Background()
		now := time.Now()
		today := now.Format("2006-01-02")

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// VERIFICAR LIMITE DIÁRIO (POR TENANT, NÃO POR API KEY!)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		collDaily := rl.db.Collection("rate_limits")
		var rateLimitDaily domain.RateLimit

		err := collDaily.FindOne(ctx, bson.M{
			"tenantId": tenantID, // ✅ POR TENANT (não por API key!)
			"date":     today,
		}).Decode(&rateLimitDaily)

		if err == mongo.ErrNoDocuments {
			// Criar novo registro
			rateLimitDaily = domain.RateLimit{
				APIKey:    tenantID, // Usando APIKey field para tenantID (legacy compatibility)
				Date:      today,
				Count:     0,
				LastReset: now,
				UpdatedAt: now,
			}
		}

		// ✅ VERIFICAR ANTES DE INCREMENTAR!
		if rateLimitDaily.Count >= config.RequestsPerDay {
			fmt.Printf("🚫 Rate Limit DIÁRIO excedido para tenant %s: %d >= %d\n", tenantID, rateLimitDaily.Count, config.RequestsPerDay)

			c.Header("X-RateLimit-Limit-Day", fmt.Sprintf("%d", config.RequestsPerDay))
			c.Header("X-RateLimit-Remaining-Day", "0")
			c.Header("X-RateLimit-Reset-Day", getNextDayTimestamp())

			c.JSON(http.StatusTooManyRequests, gin.H{
				"type":   "https://retech-core/errors/rate-limit-exceeded",
				"title":  "Rate Limit Exceeded",
				"status": http.StatusTooManyRequests,
				"detail": fmt.Sprintf("Limite de %d requests por dia excedido", config.RequestsPerDay),
			})
			c.Abort()
			return
		}

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// VERIFICAR LIMITE POR MINUTO (POR TENANT, NÃO POR API KEY!)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		collMinute := rl.db.Collection("rate_limits_minute")
		currentMinute := now.Format("2006-01-02 15:04") // YYYY-MM-DD HH:MM

		var rateLimitMinute domain.RateLimit
		err = collMinute.FindOne(ctx, bson.M{
			"tenantId": tenantID, // ✅ POR TENANT (não por API key!)
			"date":     currentMinute,
		}).Decode(&rateLimitMinute)

		if err == mongo.ErrNoDocuments {
			rateLimitMinute = domain.RateLimit{
				APIKey:    tenantID, // Usando APIKey field para tenantID (legacy compatibility)
				Date:      currentMinute,
				Count:     0,
				LastReset: now,
				UpdatedAt: now,
			}
		}

		// ✅ VERIFICAR LIMITE POR MINUTO ANTES DE INCREMENTAR!
		if rateLimitMinute.Count >= config.RequestsPerMinute {
			fmt.Printf("🚫 Rate Limit POR MINUTO excedido para tenant %s: %d >= %d\n", tenantID, rateLimitMinute.Count, config.RequestsPerMinute)

			c.Header("X-RateLimit-Limit-Minute", fmt.Sprintf("%d", config.RequestsPerMinute))
			c.Header("X-RateLimit-Remaining-Minute", "0")
			c.Header("X-RateLimit-Reset-Minute", getNextMinuteTimestamp())

			c.JSON(http.StatusTooManyRequests, gin.H{
				"type":   "https://retech-core/errors/rate-limit-exceeded",
				"title":  "Rate Limit Exceeded (Per Minute)",
				"status": http.StatusTooManyRequests,
				"detail": fmt.Sprintf("Limite de %d requests por minuto excedido. Tente novamente em alguns segundos.", config.RequestsPerMinute),
			})
			c.Abort()
			return
		}

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// INCREMENTAR CONTADORES (APÓS VERIFICAÇÃO!)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		// Incrementar contador diário
		rateLimitDaily.Count++
		rateLimitDaily.UpdatedAt = now

		opts := options.Update().SetUpsert(true)
		_, err = collDaily.UpdateOne(ctx, bson.M{
			"tenantId": tenantID, // ✅ POR TENANT
			"date":     today,
		}, bson.M{
			"$set": rateLimitDaily,
		}, opts)

		if err != nil {
			fmt.Printf("⚠️  Erro ao atualizar rate limit diário: %v\n", err)
		}

		// Incrementar contador por minuto
		rateLimitMinute.Count++
		rateLimitMinute.UpdatedAt = now

		_, err = collMinute.UpdateOne(ctx, bson.M{
			"tenantId": tenantID, // ✅ POR TENANT
			"date":     currentMinute,
		}, bson.M{
			"$set": rateLimitMinute,
		}, opts)

		if err != nil {
			fmt.Printf("⚠️  Erro ao atualizar rate limit por minuto: %v\n", err)
		}

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// ADICIONAR HEADERS DE RATE LIMIT
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		remainingDay := config.RequestsPerDay - rateLimitDaily.Count
		if remainingDay < 0 {
			remainingDay = 0
		}

		remainingMinute := config.RequestsPerMinute - rateLimitMinute.Count
		if remainingMinute < 0 {
			remainingMinute = 0
		}

		// Headers diários
		c.Header("X-RateLimit-Limit-Day", fmt.Sprintf("%d", config.RequestsPerDay))
		c.Header("X-RateLimit-Remaining-Day", fmt.Sprintf("%d", remainingDay))
		c.Header("X-RateLimit-Reset-Day", getNextDayTimestamp())

		// Headers por minuto
		c.Header("X-RateLimit-Limit-Minute", fmt.Sprintf("%d", config.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining-Minute", fmt.Sprintf("%d", remainingMinute))
		c.Header("X-RateLimit-Reset-Minute", getNextMinuteTimestamp())

		fmt.Printf("✅ Request permitida. Restante: %d/dia, %d/min\n", remainingDay, remainingMinute)

		c.Next()
	}
}

// getRateLimitConfig retorna a configuração de rate limit para um tenant.
// Ordem de resolução:
//  1. RateLimit gravado no tenant (custom/grandfathered — tenants existentes mantêm limites)
//  2. Limites do plano do tenant (Pricing v2)
//  3. Default do sistema (settings) — tenants legados sem plano nem rateLimit
//  4. Fallback hardcoded (plano free)
func (rl *RateLimiter) getRateLimitConfig(tenantID string) domain.RateLimitConfig {
	ctx := context.Background()

	// Tentar buscar tenant
	tenant, err := rl.tenants.ByTenantID(ctx, tenantID)
	if err == nil && tenant != nil {
		if tenant.RateLimit != nil {
			// Tenant tem configuração personalizada (ou grandfathered)
			return *tenant.RateLimit
		}
		if tenant.Plan != "" {
			// Limites definidos pelo plano
			return domain.PlanLimits(tenant.Plan)
		}
	}

	// Usar configuração padrão do sistema
	settings, err := rl.settings.Get(ctx)
	if err == nil && settings != nil {
		return settings.DefaultRateLimit
	}

	// Fallback para valores hardcoded se tudo falhar (plano free)
	return domain.PlanLimits(domain.PlanFree)
}

// getNextDayTimestamp retorna timestamp Unix do próximo dia (meia-noite)
func getNextDayTimestamp() string {
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return fmt.Sprintf("%d", tomorrow.Unix())
}

// getNextMinuteTimestamp retorna timestamp Unix do próximo minuto
func getNextMinuteTimestamp() string {
	now := time.Now()
	nextMinute := now.Add(time.Minute).Truncate(time.Minute)
	return fmt.Sprintf("%d", nextMinute.Unix())
}
