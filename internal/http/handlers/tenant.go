package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theretech/retech-core/internal/auth"
	"github.com/theretech/retech-core/internal/domain"
	"github.com/theretech/retech-core/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
)

type TenantHandler struct {
	apikeys *storage.APIKeysRepo
	users   *storage.UsersRepo
	tenants *storage.TenantsRepo
	db      *storage.Mongo
}

func NewTenantHandler(apikeys *storage.APIKeysRepo, users *storage.UsersRepo, tenants *storage.TenantsRepo, m *storage.Mongo) *TenantHandler {
	return &TenantHandler{
		apikeys: apikeys,
		users:   users,
		tenants: tenants,
		db:      m,
	}
}

// ListMyAPIKeys lista as API keys do tenant logado
// GET /me/apikeys
func (h *TenantHandler) ListMyAPIKeys(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	ctx := c.Request.Context()

	cursor, err := h.db.DB.Collection("api_keys").Find(ctx, bson.M{"ownerId": tenantID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Internal Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao buscar API keys",
		})
		return
	}
	defer cursor.Close(ctx)

	var keys []bson.M
	if err := cursor.All(ctx, &keys); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Internal Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao processar API keys",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"apikeys": keys,
		"total":   len(keys),
	})
}

// CreateAPIKey cria uma nova API key para o tenant logado
// POST /me/apikeys
func (h *TenantHandler) CreateAPIKey(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":   "https://retech-core/errors/validation-error",
			"title":  "Validation Error",
			"status": http.StatusBadRequest,
			"detail": err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// ✅ Gerar API key REAL (mesmo algoritmo do admin)
	keyId := uuid.NewString()
	keySecret := randomBase32Tenant(32)
	secret := os.Getenv("APIKEY_HASH_SECRET")
	hash := hashKeyTenant(secret, keyId, keySecret)

	// Validade padrão: 90 dias
	days := envIntTenant("APIKEY_TTL_DAYS", 90)
	now := time.Now().UTC()

	// Usar domain.APIKey para garantir consistência
	k := &domain.APIKey{
		KeyID:     keyId,
		KeyHash:   hash,
		OwnerID:   tenantID,
		Scopes:    []string{"geo:read"},
		ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour),
		Revoked:   false,
		CreatedAt: now,
	}

	if err := h.apikeys.Insert(ctx, k); err != nil {
		fmt.Printf("❌ Erro ao inserir API key: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Internal Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao criar API key",
		})
		return
	}

	fmt.Printf("✅ API Key criada com sucesso para tenant %s\n", tenantID)
	fmt.Printf("   keyId: %s\n", keyId)
	fmt.Printf("   Full key: %s.%s\n", keyId, keySecret)

	// ⚠️ IMPORTANTE: Retornar a chave COMPLETA apenas agora!
	c.JSON(http.StatusCreated, gin.H{
		"key":       keyId + "." + keySecret, // ← Chave completa!
		"expiresAt": k.ExpiresAt,
		"name":      req.Name,
	})
}

// RotateAPIKey rotaciona uma API key do tenant logado (revoga antiga e cria nova)
// POST /me/apikeys/:id/rotate
func (h *TenantHandler) RotateAPIKey(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	keyID := c.Param("id")
	ctx := c.Request.Context()

	// Buscar API key existente e verificar ownership
	existingKey, err := h.apikeys.ByKeyIDAny(ctx, keyID)
	if err != nil {
		fmt.Printf("❌ Erro ao buscar API key: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Erro interno",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao buscar API key",
		})
		return
	}

	if existingKey == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"type":   "https://retech-core/errors/not-found",
			"title":  "API Key não encontrada",
			"status": http.StatusNotFound,
			"detail": "API key não existe",
		})
		return
	}

	// Verificar se a key pertence ao tenant
	if existingKey.OwnerID != tenantID {
		c.JSON(http.StatusForbidden, gin.H{
			"type":   "https://retech-core/errors/forbidden",
			"title":  "Acesso negado",
			"status": http.StatusForbidden,
			"detail": "Você não tem permissão para rotacionar esta API key",
		})
		return
	}

	// Revogar a chave antiga
	if err := h.apikeys.Revoke(ctx, keyID); err != nil {
		fmt.Printf("❌ Erro ao revogar API key: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Erro interno",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao revogar chave antiga",
		})
		return
	}

	// Criar nova chave com os mesmos dados
	keyId := uuid.NewString()
	keySecret := randomBase32Tenant(32)
	secret := os.Getenv("APIKEY_HASH_SECRET")
	hash := hashKeyTenant(secret, keyId, keySecret)

	// Calcular nova data de expiração (mesmo período da chave original)
	now := time.Now().UTC()
	days := int(existingKey.ExpiresAt.Sub(existingKey.CreatedAt).Hours() / 24)
	if days <= 0 {
		days = envIntTenant("APIKEY_TTL_DAYS", 90)
	}

	newKey := &domain.APIKey{
		KeyID:     keyId,
		KeyHash:   hash,
		OwnerID:   tenantID,
		Scopes:    existingKey.Scopes,
		ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour),
		Revoked:   false,
		CreatedAt: now,
	}

	if err := h.apikeys.Insert(ctx, newKey); err != nil {
		fmt.Printf("❌ Erro ao criar nova API key: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Erro interno",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao criar nova chave",
		})
		return
	}

	fmt.Printf("✅ API Key rotacionada com sucesso para tenant %s\n", tenantID)
	fmt.Printf("   Old keyId: %s\n", keyID)
	fmt.Printf("   New keyId: %s\n", keyId)

	// Retornar a nova chave (apenas uma vez!)
	c.JSON(http.StatusCreated, gin.H{
		"key":       keyId + "." + keySecret,
		"expiresAt": newKey.ExpiresAt,
		"message":   "API key rotacionada com sucesso",
	})
}

// DeleteAPIKey deleta uma API key do tenant logado
// DELETE /me/apikeys/:id
func (h *TenantHandler) DeleteAPIKey(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	keyID := c.Param("id")

	ctx := c.Request.Context()

	// Verificar se a key pertence ao tenant
	var apikey bson.M
	err := h.db.DB.Collection("api_keys").FindOne(ctx, bson.M{
		"keyId":   keyID,
		"ownerId": tenantID,
	}).Decode(&apikey)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"type":   "https://retech-core/errors/not-found",
			"title":  "Not Found",
			"status": http.StatusNotFound,
			"detail": "API key não encontrada",
		})
		return
	}

	// Revogar (soft delete)
	_, err = h.db.DB.Collection("api_keys").UpdateOne(ctx, bson.M{"keyId": keyID}, bson.M{
		"$set": bson.M{"revoked": true},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/internal-error",
			"title":  "Internal Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao revogar API key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revogada com sucesso",
	})
}

// GetMyUsage retorna uso da API do tenant logado
// GetMyConfig retorna configurações do tenant para a documentação
// GET /me/config
func (h *TenantHandler) GetMyConfig(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	ctx := c.Request.Context()

	// Buscar tenant
	tenant, err := h.tenants.ByTenantID(ctx, tenantID)
	if err != nil || tenant == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"type":   "https://retech-core/errors/not-found",
			"title":  "Not Found",
			"status": http.StatusNotFound,
			"detail": "Tenant não encontrado",
		})
		return
	}

	// API Base URL (pode vir de env ou ser hardcoded)
	apiBaseURL := os.Getenv("API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = "https://api-core.theretech.com.br"
	}

	// Rate Limit (fallback: limites do plano)
	planLimits := domain.PlanLimits(tenant.Plan)
	dailyLimit := planLimits.RequestsPerDay
	minuteLimit := planLimits.RequestsPerMinute
	if tenant.RateLimit != nil {
		dailyLimit = tenant.RateLimit.RequestsPerDay
		minuteLimit = tenant.RateLimit.RequestsPerMinute
	}

	c.JSON(http.StatusOK, gin.H{
		"apiBaseURL": apiBaseURL,
		"rateLimit": gin.H{
			"requestsPerDay":    dailyLimit,
			"requestsPerMinute": minuteLimit,
		},
		"endpoints": []gin.H{
			{
				"category": "CEP",
				"items": []gin.H{
					{
						"method":      "GET",
						"path":        "/cep/:codigo",
						"description": "Consulta CEP com cache (ViaCEP + Brasil API)",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/cep/buscar",
						"description": "🆕 Busca reversa: encontra CEP por endereço (UF, cidade, logradouro). Aceita acentos! Até 50 resultados.",
						"available":   true,
					},
				},
			},
			{
				"category": "CNPJ",
				"items": []gin.H{
					{
						"method":      "GET",
						"path":        "/cnpj/:numero",
						"description": "Consulta CNPJ da Receita Federal (Brasil API + ReceitaWS)",
						"available":   true,
					},
				},
			},
			{
				"category": "Geografia",
				"items": []gin.H{
					{
						"method":      "GET",
						"path":        "/geo/ufs",
						"description": "Lista todos os estados brasileiros",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/geo/ufs/:sigla",
						"description": "Detalhes de um estado específico",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/geo/municipios",
						"description": "Lista todos os municípios",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/geo/municipios/:uf",
						"description": "Municípios de um estado",
						"available":   true,
					},
				},
			},
			{
				"category": "Artigos Penais",
				"items": []gin.H{
					{
						"method":      "GET",
						"path":        "/penal/artigos",
						"description": "🆕 Lista artigos penais (autocomplete/Select2). Filtros: q (busca), tipo (crime/contravencao), legislacao (CP/LCP)",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/penal/artigos/:codigo",
						"description": "🆕 Busca artigo específico por código (ex: 121, 121.1, 121.1.I)",
						"available":   true,
					},
					{
						"method":      "GET",
						"path":        "/penal/search",
						"description": "🆕 Busca artigos por texto (descrição ou texto completo). Ideal para busca inteligente",
						"available":   true,
					},
				},
			},
		},
	})
}

// GET /me/usage
func (h *TenantHandler) GetMyUsage(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	ctx := c.Request.Context()

	// ✅ FIX: Usar timezone Brasília (consistência com admin)
	now := time.Now()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	nowBrasilia := now.In(loc)
	today := nowBrasilia.Format("2006-01-02")
	startOfMonth := nowBrasilia.Format("2006-01")

	// Total de requests
	totalRequests, _ := h.db.DB.Collection("api_usage_logs").CountDocuments(ctx, bson.M{"tenantId": tenantID})

	// Requests hoje (timezone Brasília)
	requestsToday, _ := h.db.DB.Collection("api_usage_logs").CountDocuments(ctx, bson.M{
		"tenantId": tenantID,
		"date":     today,
	})

	// Requests mês (timezone Brasília)
	requestsMonth, _ := h.db.DB.Collection("api_usage_logs").CountDocuments(ctx, bson.M{
		"tenantId": tenantID,
		"date":     bson.M{"$regex": "^" + startOfMonth},
	})

	// Buscar limite diário do tenant (SEMPRE tem rateLimit salvo agora!)
	tenant, err := h.tenants.ByTenantID(ctx, tenantID)
	dailyLimit := domain.PlanLimits(domain.PlanFree).RequestsPerDay // Fallback (plano free)

	if err != nil || tenant == nil {
		fmt.Printf("⚠️ [GetMyUsage] Tenant não encontrado! Usando fallback.\n")
	} else if tenant.RateLimit == nil {
		dailyLimit = domain.PlanLimits(tenant.Plan).RequestsPerDay
		fmt.Printf("⚠️ [GetMyUsage] Tenant SEM rateLimit! Usando limites do plano '%s'.\n", tenant.Plan)
	} else {
		dailyLimit = tenant.RateLimit.RequestsPerDay
		fmt.Printf("✅ [GetMyUsage] dailyLimit do tenant: %d/dia\n", dailyLimit)
	}

	remaining := dailyLimit - requestsToday
	if remaining < 0 {
		remaining = 0
	}

	// Por dia (últimos 7 dias)
	pipeline := []bson.M{
		{"$match": bson.M{
			"tenantId":  tenantID,
			"timestamp": bson.M{"$gte": time.Now().AddDate(0, 0, -7)},
		}},
		{"$group": bson.M{
			"_id":   "$date",
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"_id": 1}},
	}

	cursor, _ := h.db.DB.Collection("api_usage_logs").Aggregate(ctx, pipeline)
	defer cursor.Close(ctx)

	var byDay []bson.M
	cursor.All(ctx, &byDay)

	// Por endpoint
	pipelineEndpoints := []bson.M{
		{"$match": bson.M{"tenantId": tenantID}},
		{"$group": bson.M{
			"_id":   "$endpoint",
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"count": -1}},
		{"$limit": 10},
	}

	cursor2, _ := h.db.DB.Collection("api_usage_logs").Aggregate(ctx, pipelineEndpoints)
	defer cursor2.Close(ctx)

	var byEndpoint []bson.M
	cursor2.All(ctx, &byEndpoint)

	// Por API (nova métrica!)
	pipelineAPIs := []bson.M{
		{"$match": bson.M{"tenantId": tenantID}},
		{"$group": bson.M{
			"_id":             "$apiName",
			"count":           bson.M{"$sum": 1},
			"avgResponseTime": bson.M{"$avg": "$responseTime"},
		}},
		{"$sort": bson.M{"count": -1}},
	}

	cursor3, _ := h.db.DB.Collection("api_usage_logs").Aggregate(ctx, pipelineAPIs)
	defer cursor3.Close(ctx)

	var byAPI []bson.M
	cursor3.All(ctx, &byAPI)

	c.JSON(http.StatusOK, gin.H{
		"totalRequests":  totalRequests,
		"requestsToday":  requestsToday,
		"requestsMonth":  requestsMonth,
		"dailyLimit":     dailyLimit,
		"remaining":      remaining,
		"percentageUsed": float64(requestsToday) / float64(dailyLimit) * 100,
		"byDay":          byDay,
		"byEndpoint":     byEndpoint,
		"byAPI":          byAPI, // ✅ NOVO: Breakdown por API
	})
}

// GetMyStats retorna métricas rápidas para o dashboard
// GET /me/stats
func (h *TenantHandler) GetMyStats(c *gin.Context) {
	tenantID := auth.GetTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type":   "https://retech-core/errors/unauthorized",
			"title":  "Unauthorized",
			"status": http.StatusUnauthorized,
			"detail": "Tenant ID não encontrado",
		})
		return
	}

	ctx := c.Request.Context()

	// ✅ FIX: Usar timezone Brasília (consistência)
	now := time.Now()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	nowBrasilia := now.In(loc)
	today := nowBrasilia.Format("2006-01-02")

	// 1. Total de API Keys ativas
	activeKeys, _ := h.apikeys.CountByOwner(ctx, tenantID)

	// 2. Buscar rate limit atual do tenant (timezone Brasília)
	var rateLimit domain.RateLimit
	err := h.db.DB.Collection("rate_limits").FindOne(ctx, bson.M{
		"tenantId": tenantID,
		"date":     today,
	}).Decode(&rateLimit)

	requestsToday := int64(0)
	if err == nil {
		requestsToday = int64(rateLimit.Count)
	}

	// 3. Buscar tenant (SEMPRE tem rateLimit salvo agora!)
	tenant, err := h.tenants.ByTenantID(ctx, tenantID)
	dailyLimit := domain.PlanLimits(domain.PlanFree).RequestsPerDay // Fallback (plano free, não deveria acontecer)

	fmt.Printf("🔍 [GetMyStats] TenantID: %s\n", tenantID)

	if err != nil || tenant == nil {
		fmt.Printf("⚠️ [GetMyStats] Tenant não encontrado! Usando fallback.\n")
	} else if tenant.RateLimit == nil {
		dailyLimit = domain.PlanLimits(tenant.Plan).RequestsPerDay
		fmt.Printf("⚠️ [GetMyStats] Tenant SEM rateLimit! Usando limites do plano '%s'.\n", tenant.Plan)
	} else {
		dailyLimit = tenant.RateLimit.RequestsPerDay
		fmt.Printf("✅ [GetMyStats] dailyLimit do tenant: %d/dia, %d/min\n",
			tenant.RateLimit.RequestsPerDay, tenant.RateLimit.RequestsPerMinute)
	}

	// 4. Calcular remaining e percentage
	remaining := dailyLimit - requestsToday
	if remaining < 0 {
		remaining = 0
	}

	percentage := float64(0)
	if dailyLimit > 0 {
		percentage = (float64(requestsToday) / float64(dailyLimit)) * 100
	}

	// 5. Requests do mês (aproximado via rate_limits)
	startOfMonth := time.Now().Format("2006-01")
	var rateLimitsMonth []domain.RateLimit
	cursor, _ := h.db.DB.Collection("rate_limits").Find(ctx, bson.M{
		"tenantId": tenantID,
		"date":     bson.M{"$regex": "^" + startOfMonth},
	})
	defer cursor.Close(ctx)
	cursor.All(ctx, &rateLimitsMonth)

	requestsMonth := int64(0)
	for _, rl := range rateLimitsMonth {
		requestsMonth += int64(rl.Count)
	}

	c.JSON(http.StatusOK, gin.H{
		"activeKeys":     activeKeys,
		"requestsToday":  requestsToday,
		"requestsMonth":  requestsMonth,
		"dailyLimit":     dailyLimit,
		"remaining":      remaining,
		"percentageUsed": percentage,
	})
}

// ========================================
// Funções auxiliares (mesmas do apikey.go)
// ========================================

func hashKeyTenant(secret, keyId, keySecret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(keyId + "." + keySecret))
	return hex.EncodeToString(h.Sum(nil))
}

func envIntTenant(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func randomBase32Tenant(n int) string {
	const a = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	sb := strings.Builder{}
	for i := 0; i < n; i++ {
		sb.WriteByte(a[rand.Intn(len(a))])
	}
	return sb.String()
}
