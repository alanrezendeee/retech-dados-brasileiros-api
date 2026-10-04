package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theretech/retech-core/internal/cache"
	"github.com/theretech/retech-core/internal/domain"
	"github.com/theretech/retech-core/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// penalCacheVersion versiona TODAS as chaves Redis da API Penal.
// Ao alterar o seed (novos dispositivos, correções de texto, remoções), incrementar
// a versão invalida de uma só vez todas as respostas penais cacheadas (listas,
// artigos individuais e buscas), sem precisar varrer/limpar o Redis manualmente.
// As chaves continuam começando com "penal:" para que redis_stats.go as contabilize.
const penalCacheVersion = "v2"

// penalNiveisValidos lista os valores aceitos para o filtro `nivel`
var penalNiveisValidos = map[string]bool{
	"artigo":    true,
	"paragrafo": true,
	"inciso":    true,
	"alinea":    true,
}

type PenalHandler struct {
	db    *storage.Mongo
	redis interface{} // interface{} para permitir nil (graceful degradation)
}

func NewPenalHandler(db *storage.Mongo, redis interface{}) *PenalHandler {
	return &PenalHandler{
		db:    db,
		redis: redis,
	}
}

// ListArtigos retorna todos os artigos penais (para autocomplete)
// GET /penal/artigos
func (h *PenalHandler) ListArtigos(c *gin.Context) {
	ctx := c.Request.Context()
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	tipo := c.Query("tipo")             // "crime", "contravencao", "disposicao", "revogado" ou vazio (todos)
	legislacao := c.Query("legislacao") // "CP", "LCP", "Lei 8.137/1990", etc
	nivel := strings.ToLower(strings.TrimSpace(c.Query("nivel")))

	// Validar nivel (se informado)
	if nivel != "" && !penalNiveisValidos[nivel] {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":   "https://retech-core/errors/validation",
			"title":  "Invalid Query",
			"status": http.StatusBadRequest,
			"detail": fmt.Sprintf("Parâmetro 'nivel' inválido: '%s'. Valores aceitos: artigo, paragrafo, inciso, alinea", nivel),
		})
		return
	}

	semFiltros := query == "" && tipo == "" && legislacao == "" && nivel == ""

	// Criar chave de cache (versionada — ver penalCacheVersion)
	cacheKey := fmt.Sprintf("penal:%s:artigos:%s:%s:%s:%s", penalCacheVersion, query, tipo, legislacao, nivel)
	if semFiltros {
		cacheKey = fmt.Sprintf("penal:%s:artigos:all", penalCacheVersion)
	}

	// 🗄️ Collection e filtro (montados antes do cache para reutilizar na validação do glossário)
	collection := h.db.DB.Collection("penal_artigos")

	filter := bson.M{}

	// Filtro por tipo
	if tipo != "" {
		filter["tipo"] = tipo
	}

	// Filtro por legislação
	if legislacao != "" {
		filter["legislacao"] = legislacao
	}

	// Filtro por nível (artigo | paragrafo | inciso | alinea)
	if nivel != "" {
		filter["nivel"] = nivel
	}

	// Filtro por busca (texto)
	if query != "" {
		filter["busca"] = bson.M{"$regex": query, "$options": "i"}
	}

	// ⚡ CACHE REDIS (ultra-rápido, <1ms)
	// IMPORTANTE: Para glossário completo (sem filtros), validar que o cache reflete o banco:
	// compara meta.total do cache com um CountDocuments ao vivo (barato, coleção indexada).
	// Se divergir (seed alterado sem bump de penalCacheVersion), invalida e segue para o Mongo.
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			cachedJSON, err := redisClient.Get(ctx, cacheKey)
			if err == nil && cachedJSON != "" {
				if semFiltros {
					var cached struct {
						Meta struct {
							Total int64 `json:"total"`
						} `json:"meta"`
					}
					cacheValido := false
					if json.Unmarshal([]byte(cachedJSON), &cached) == nil {
						liveTotal, countErr := collection.CountDocuments(ctx, filter)
						if countErr == nil && liveTotal == cached.Meta.Total {
							cacheValido = true
						}
					}
					if cacheValido {
						c.Header("Content-Type", "application/json")
						c.String(http.StatusOK, cachedJSON)
						return // ⚡ <1ms!
					}
					// Cache desatualizado (ou inválido), remover e buscar do banco
					redisClient.Del(ctx, cacheKey)
				} else {
					// Para buscas com filtros, sempre usar cache
					c.Header("Content-Type", "application/json")
					c.String(http.StatusOK, cachedJSON)
					return // ⚡ <1ms!
				}
			}
		}
	}

	// 🗄️ BUSCAR DO MONGODB
	// Ordenação global pelo campo `ordem` (CP em ordem de artigo, depois leis especiais)
	findOptions := options.Find().
		SetSort(bson.D{{Key: "ordem", Value: 1}})

	// Busca textual (autocomplete) é limitada a 100 resultados.
	// Filtros estruturais (tipo/legislacao/nivel) e a busca completa retornam todos os
	// dispositivos, para que o glossário filtrado (ex.: nivel=artigo, legislacao=CP) seja íntegro.
	if query != "" {
		findOptions = findOptions.SetLimit(100)
	}

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/database-error",
			"title":  "Database Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao buscar artigos penais",
		})
		return
	}
	defer cursor.Close(ctx)

	var artigos []domain.ArtigoPenal
	if err := cursor.All(ctx, &artigos); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/database-error",
			"title":  "Database Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao processar resultados",
		})
		return
	}

	// Converter para formato de resposta (autocomplete)
	results := make([]domain.PenalResponse, 0, len(artigos))
	for _, artigo := range artigos {
		results = append(results, domain.PenalResponse{
			Codigo:          artigo.Codigo,
			CodigoFormatado: artigo.CodigoFormatado,
			Descricao:       artigo.Descricao,
			Tipo:            artigo.Tipo,
			Nivel:           artigo.Nivel,
			Legislacao:      artigo.Legislacao,
			LegislacaoNome:  artigo.LegislacaoNome,
			IdUnico:         artigo.IdUnico,
		})
	}

	response := gin.H{
		"success": true,
		"code":    "OK",
		"data":    results,
		"meta": gin.H{
			"total": len(results),
			"query": query,
			"nivel": nivel,
		},
	}

	// ⚡ Salvar no Redis (cache permanente para dados fixos - 365 dias)
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			// Cache permanente (365 dias) para dados fixos
			redisClient.Set(ctx, cacheKey, response, 365*24*time.Hour)
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetArtigo retorna um artigo específico por código
// GET /penal/artigos/:codigo
// Aceita:
//   - Código simples: "121", "121.2", "121.2.I", "121.2.VII.a" (busca primeiro no CP, depois em outras legislações;
//     se o mesmo código existir em várias legislações retorna 300 Multiple Choices)
//   - ID único: "CP:121", "DRG:33", "AMB:54", "OTE:1.I" (prefixo curto da legislação + código)
func (h *PenalHandler) GetArtigo(c *gin.Context) {
	ctx := c.Request.Context()
	codigo := c.Param("codigo")

	// Quando usar *codigo, o Gin adiciona / no início, remover se necessário
	if strings.HasPrefix(codigo, "/") {
		codigo = codigo[1:]
	}

	// Decodificar URL (importante para códigos com :, /, etc)
	// O Gin já decodifica automaticamente, mas vamos garantir
	if decoded, err := url.PathUnescape(codigo); err == nil {
		codigo = decoded
	}

	// Normalizar código (remover espaços)
	codigo = strings.TrimSpace(codigo)
	codigoNormalizado := strings.ToLower(codigo)

	// Criar chave de cache
	cacheKey := fmt.Sprintf("penal:%s:artigo:%s", penalCacheVersion, codigoNormalizado)

	// ⚡ CACHE REDIS
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			cachedJSON, err := redisClient.Get(ctx, cacheKey)
			if err == nil && cachedJSON != "" {
				c.Header("Content-Type", "application/json")
				c.String(http.StatusOK, cachedJSON)
				return
			}
		}
	}

	// 🗄️ BUSCAR DO MONGODB
	collection := h.db.DB.Collection("penal_artigos")

	var filter bson.M
	var artigo domain.ArtigoPenal

	// Verificar se é formato idUnico (CODIGO:ARTIGO)
	if strings.Contains(codigo, ":") {
		// Busca por idUnico (exato) - formato: "CP:121", "DRG:33", etc
		filter = bson.M{"idUnico": codigo}
		err := collection.FindOne(ctx, filter).Decode(&artigo)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{
					"type":   "https://retech-core/errors/not-found",
					"title":  "Artigo Not Found",
					"status": http.StatusNotFound,
					"detail": fmt.Sprintf("Artigo %s não encontrado. Use o formato 'CODIGO:ARTIGO' (ex: 'CP:121', 'DRG:33', 'AMB:54')", codigo),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"type":   "https://retech-core/errors/database-error",
				"title":  "Database Error",
				"status": http.StatusInternalServerError,
				"detail": "Erro ao buscar artigo",
			})
			return
		}
	} else {
		// Busca por código simples
		// Primeiro tenta no CP (legislação mais comum)
		filter = bson.M{"codigo": codigo, "legislacao": "CP"}
		err := collection.FindOne(ctx, filter).Decode(&artigo)

		if err == mongo.ErrNoDocuments {
			// Se não encontrou no CP, busca em qualquer legislação
			filter = bson.M{"codigo": codigo}
			cursor, err := collection.Find(ctx, filter)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"type":   "https://retech-core/errors/database-error",
					"title":  "Database Error",
					"status": http.StatusInternalServerError,
					"detail": "Erro ao buscar artigo",
				})
				return
			}
			defer cursor.Close(ctx)

			var artigos []domain.ArtigoPenal
			if err := cursor.All(ctx, &artigos); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"type":   "https://retech-core/errors/database-error",
					"title":  "Database Error",
					"status": http.StatusInternalServerError,
					"detail": "Erro ao processar resultados",
				})
				return
			}

			if len(artigos) == 0 {
				c.JSON(http.StatusNotFound, gin.H{
					"type":   "https://retech-core/errors/not-found",
					"title":  "Artigo Not Found",
					"status": http.StatusNotFound,
					"detail": fmt.Sprintf("Artigo %s não encontrado. Use o formato 'CODIGO:ARTIGO' para especificar a legislação (ex: 'CP:121', 'DRG:33', 'AMB:54')", codigo),
				})
				return
			}

			if len(artigos) > 1 {
				// Múltiplos artigos encontrados - retornar lista com sugestão
				legislacoes := make([]string, len(artigos))
				for i, art := range artigos {
					legislacoes[i] = art.Legislacao
				}
				c.JSON(http.StatusMultipleChoices, gin.H{
					"type":   "https://retech-core/errors/multiple-choices",
					"title":  "Multiple Articles Found",
					"status": http.StatusMultipleChoices,
					"detail": fmt.Sprintf("Múltiplos artigos encontrados com código '%s'. Use o formato 'CODIGO:ARTIGO' para especificar", codigo),
					"data": gin.H{
						"codigo":      codigo,
						"artigos":     artigos,
						"legislacoes": legislacoes,
						"sugestao":    "Use: /penal/artigos/CODIGO:ARTIGO (ex: /penal/artigos/CP:121 ou /penal/artigos/DRG:33)",
					},
				})
				return
			}

			artigo = artigos[0]
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"type":   "https://retech-core/errors/database-error",
				"title":  "Database Error",
				"status": http.StatusInternalServerError,
				"detail": "Erro ao buscar artigo",
			})
			return
		}
	}

	response := gin.H{
		"success": true,
		"code":    "OK",
		"data":    artigo,
	}

	// ⚡ Salvar no Redis (cache permanente - 365 dias)
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			redisClient.Set(ctx, cacheKey, response, 365*24*time.Hour)
		}
	}

	c.JSON(http.StatusOK, response)
}

// SearchArtigos busca artigos por texto (descrição)
// GET /penal/search?q=texto
func (h *PenalHandler) SearchArtigos(c *gin.Context) {
	ctx := c.Request.Context()
	query := strings.TrimSpace(c.Query("q"))

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type":   "https://retech-core/errors/validation",
			"title":  "Invalid Query",
			"status": http.StatusBadRequest,
			"detail": "Parâmetro 'q' é obrigatório",
		})
		return
	}

	// Criar chave de cache
	queryLower := strings.ToLower(query)
	cacheKey := fmt.Sprintf("penal:%s:search:%s", penalCacheVersion, queryLower)

	// ⚡ CACHE REDIS
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			cachedJSON, err := redisClient.Get(ctx, cacheKey)
			if err == nil && cachedJSON != "" {
				c.Header("Content-Type", "application/json")
				c.String(http.StatusOK, cachedJSON)
				return
			}
		}
	}

	// 🗄️ BUSCAR DO MONGODB
	collection := h.db.DB.Collection("penal_artigos")

	// Busca em múltiplos campos
	filter := bson.M{
		"$or": []bson.M{
			{"descricao": bson.M{"$regex": query, "$options": "i"}},
			{"textoCompleto": bson.M{"$regex": query, "$options": "i"}},
			{"busca": bson.M{"$regex": queryLower, "$options": "i"}},
		},
	}

	findOptions := options.Find().
		SetSort(bson.D{{Key: "ordem", Value: 1}}).
		SetLimit(50)

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/database-error",
			"title":  "Database Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao buscar artigos",
		})
		return
	}
	defer cursor.Close(ctx)

	var artigos []domain.ArtigoPenal
	if err := cursor.All(ctx, &artigos); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type":   "https://retech-core/errors/database-error",
			"title":  "Database Error",
			"status": http.StatusInternalServerError,
			"detail": "Erro ao processar resultados",
		})
		return
	}

	// Converter para formato de resposta
	results := make([]domain.PenalResponse, 0, len(artigos))
	for _, artigo := range artigos {
		results = append(results, domain.PenalResponse{
			Codigo:          artigo.Codigo,
			CodigoFormatado: artigo.CodigoFormatado,
			Descricao:       artigo.Descricao,
			Tipo:            artigo.Tipo,
			Nivel:           artigo.Nivel,
			Legislacao:      artigo.Legislacao,
			LegislacaoNome:  artigo.LegislacaoNome,
			IdUnico:         artigo.IdUnico,
		})
	}

	response := gin.H{
		"success": true,
		"code":    "OK",
		"data":    results,
		"meta": gin.H{
			"total": len(results),
			"query": query,
		},
	}

	// ⚡ Salvar no Redis (cache 24h para buscas)
	if h.redis != nil {
		if redisClient, ok := h.redis.(*cache.RedisClient); ok {
			redisClient.Set(ctx, cacheKey, response, 24*time.Hour)
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetCacheStats retorna estatísticas do cache de Artigos Penais
// GET /admin/cache/penal/stats
func (h *PenalHandler) GetCacheStats(c *gin.Context) {
	ctx := c.Request.Context()
	collection := h.db.DB.Collection("penal_artigos")

	// Total de artigos penais no banco (seed permanente)
	totalCached, _ := collection.CountDocuments(ctx, bson.M{})

	// Artigos adicionados nas últimas 24h (caso tenha novos artigos)
	yesterday := time.Now().Add(-24 * time.Hour)
	recentCached, _ := collection.CountDocuments(ctx, bson.M{
		"createdAt": bson.M{"$gte": yesterday},
	})

	// Penal v2: distribuição por tipo e por nível
	totalPorTipo := make(map[string]int64, 4)
	for _, tipo := range []string{"crime", "contravencao", "disposicao", "revogado"} {
		n, _ := collection.CountDocuments(ctx, bson.M{"tipo": tipo})
		totalPorTipo[tipo] = n
	}

	totalPorNivel := make(map[string]int64, 4)
	for _, nivel := range []string{"artigo", "paragrafo", "inciso", "alinea"} {
		n, _ := collection.CountDocuments(ctx, bson.M{"nivel": nivel})
		totalPorNivel[nivel] = n
	}

	c.JSON(http.StatusOK, gin.H{
		"totalCached":   totalCached,
		"recentCached":  recentCached, // últimas 24h
		"cacheEnabled":  true,         // Sempre habilitado (dados fixos)
		"cacheTTLDays":  365,          // Cache permanente (1 ano)
		"autoCleanup":   false,        // Não limpa automaticamente (dados fixos)
		"totalPorTipo":  totalPorTipo,
		"totalPorNivel": totalPorNivel,
		"cacheVersion":  penalCacheVersion,
	})
}
