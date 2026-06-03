package http

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/theretech/retech-core/internal/auth"
	"github.com/theretech/retech-core/internal/cepdb"
	"github.com/theretech/retech-core/internal/http/handlers"
	"github.com/theretech/retech-core/internal/middleware"
	"github.com/theretech/retech-core/internal/storage"
)

func NewRouter(
	log zerolog.Logger,
	m *storage.Mongo,
	redisClient interface{}, // interface{} para permitir nil (graceful degradation)
	cepDB *cepdb.DB,         // nil se CEPDB_URL não configurado (graceful degradation)
	health *handlers.HealthHandler,
	apikeys *storage.APIKeysRepo,
	tenants *storage.TenantsRepo,
	users *storage.UsersRepo,
	estados *storage.EstadosRepo,
	municipios *storage.MunicipiosRepo,
	settings *storage.SettingsRepo,
	activityLogs *storage.ActivityLogsRepo,
	jwtService *auth.JWTService,
) *gin.Engine {
	r := gin.New()

	// 🌐 CORS DINÂMICO (lê de admin/settings)
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		origin := c.Request.Header.Get("Origin")
		path := c.Request.URL.Path

		// 📋 Rotas públicas SEMPRE têm CORS (independente do settings)
		publicRoutes := []string{
			"/health",
			"/version",
			"/docs",
			"/openapi.yaml",
			"/public/",
		}

		isPublicRoute := false
		for _, route := range publicRoutes {
			if len(path) >= len(route) && path[:len(route)] == route {
				isPublicRoute = true
				break
			}
		}

		// Se é rota pública, sempre permite CORS
		if isPublicRoute {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-API-Key, Cache-Control, Pragma, Expires, X-Browser-Fingerprint, X-Client-IP")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
			return
		}

		// 🔒 Rotas protegidas: verificar settings
		sysSettings, err := settings.Get(ctx)

		// Se erro ao buscar settings, não adiciona CORS (seguro)
		if err != nil {
			c.Next()
			return
		}

		// Se CORS desabilitado, não adiciona headers para rotas protegidas
		if !sysSettings.CORS.Enabled {
			c.Next()
			return
		}

		// Verificar se origin está na lista permitida
		allowed := false
		for _, allowedOrigin := range sysSettings.CORS.AllowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-API-Key, Cache-Control, Pragma, Expires, X-Browser-Fingerprint, X-Client-IP")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Solução baseada no Stack Overflow: configurar para não reutilizar conexões
	r.Use(func(c *gin.Context) {
		fmt.Printf("Middleware global chamado para: %s %s\n", c.Request.Method, c.Request.URL.Path)
		c.Header("Connection", "close")
		c.Next()
	})

	// Middlewares globais
	rateLimiter := middleware.NewRateLimiter(m.DB, tenants, settings)
	playgroundRateLimiter := middleware.NewPlaygroundRateLimiter(m.DB, settings)
	usageLogger := middleware.NewUsageLogger(m.DB)
	maintenanceMiddleware := middleware.NewMaintenanceMiddleware(settings)

	// Rotas públicas (sem autenticação e sem manutenção)
	r.GET("/health", health.Get)
	r.GET("/version", handlers.Version)
	r.GET("/docs", handlers.DocsHTML)
	r.GET("/openapi.yaml", handlers.OpenAPIYAML)

	// Public endpoints
	publicSettingsHandler := handlers.NewSettingsHandler(settings, activityLogs)
	r.GET("/public/contact", publicSettingsHandler.GetPublicContact)

	// Playground status (público, sem autenticação)
	playgroundHandler := handlers.NewPlaygroundHandler(settings)
	r.GET("/public/playground/status", playgroundHandler.GetStatus)

	// Public playground/tools endpoints (sem API Key, rate limit por IP)
	cepHandler := handlers.NewCEPHandler(m, redisClient, cepDB, settings)
	cnpjHandler := handlers.NewCNPJHandler(m, redisClient, settings)
	geoHandler := handlers.NewGeoHandler(estados, municipios, redisClient)
	penalHandler := handlers.NewPenalHandler(m, redisClient)

	// 🔒 ROTAS PÚBLICAS COM SEGURANÇA MULTI-CAMADA
	// API Key Demo (obrigatória) + Scopes + Rate limiting por IP + Fingerprinting + Throttling
	publicGroup := r.Group("/public")
	{
		// CEP (requer scope 'cep')
		publicGroup.GET("/cep/:codigo",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "cep"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			cepHandler.GetCEP,
		)

		// CEP Search - Busca reversa (requer scope 'cep')
		publicGroup.GET("/cep/buscar",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "cep"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			cepHandler.SearchCEP,
		)

		// CNPJ (requer scope 'cnpj')
		publicGroup.GET("/cnpj/:numero",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "cnpj"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			cnpjHandler.GetCNPJ,
		)

		// GEO (requer scope 'geo')
		publicGroup.GET("/geo/ufs",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "geo"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			geoHandler.ListUFs,
		)

		publicGroup.GET("/geo/ufs/:sigla",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "geo"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			geoHandler.GetUF,
		)

		// PENAL (requer scope 'penal')
		publicGroup.GET("/penal/artigos",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "penal"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			penalHandler.ListArtigos,
		)

		// Usar *codigo para permitir códigos com : (ex: DRG:33)
		publicGroup.GET("/penal/artigos/*codigo",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "penal"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			penalHandler.GetArtigo,
		)

		publicGroup.GET("/penal/search",
			auth.AuthAPIKey(apikeys),
			auth.RequireScope(apikeys, "penal"), // ✅ Valida scope!
			playgroundRateLimiter.Middleware(),
			usageLogger.Middleware(),
			penalHandler.SearchArtigos,
		)
	}

	// Auth endpoints (públicos)
	authHandler := handlers.NewAuthHandler(users, tenants, apikeys, activityLogs, settings, jwtService)
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/refresh", authHandler.RefreshToken)
		authGroup.GET("/me", auth.AuthJWT(jwtService), authHandler.Me)
	}

	// GEO endpoints (protegidos por API Key + rate limit + logging + manutenção + scopes)
	geoGroup := r.Group("/geo")
	geoGroup.Use(
		maintenanceMiddleware.Middleware(), // Verifica manutenção
		auth.AuthAPIKey(apikeys),           // Requer API Key válida
		auth.RequireScope(apikeys, "geo"),  // ✅ Verifica scope 'geo' ou 'all'
		rateLimiter.Middleware(),           // Aplica rate limiting
		usageLogger.Middleware(),           // Loga uso
	)
	{
		geoGroup.GET("/ufs", geoHandler.ListUFs)
		geoGroup.GET("/ufs/:sigla", geoHandler.GetUF)
		geoGroup.GET("/municipios", geoHandler.ListMunicipios)
		geoGroup.GET("/municipios/:uf", geoHandler.ListMunicipiosByUF)
		geoGroup.GET("/municipios/id/:id", geoHandler.GetMunicipio)
	}

	// CEP endpoints (protegidos por API Key + rate limit + logging + manutenção + scopes)
	cepGroup := r.Group("/cep")
	cepGroup.Use(
		maintenanceMiddleware.Middleware(), // Verifica manutenção
		auth.AuthAPIKey(apikeys),           // Requer API Key válida
		auth.RequireScope(apikeys, "cep"),  // ✅ Verifica scope 'cep' ou 'all'
		rateLimiter.Middleware(),           // Aplica rate limiting
		usageLogger.Middleware(),           // Loga uso
	)
	{
		cepGroup.GET("/:codigo", cepHandler.GetCEP)
		cepGroup.GET("/buscar", cepHandler.SearchCEP) // Busca reversa
	}

	// CNPJ endpoints (protegidos por API Key + rate limit + logging + manutenção + scopes)
	cnpjGroup := r.Group("/cnpj")
	cnpjGroup.Use(
		maintenanceMiddleware.Middleware(), // Verifica manutenção
		auth.AuthAPIKey(apikeys),           // Requer API Key válida
		auth.RequireScope(apikeys, "cnpj"), // ✅ Verifica scope 'cnpj' ou 'all'
		rateLimiter.Middleware(),           // Aplica rate limiting
		usageLogger.Middleware(),           // Loga uso
	)
	{
		cnpjGroup.GET("/:numero", cnpjHandler.GetCNPJ)
	}

	// PENAL endpoints (protegidos por API Key + rate limit + logging + manutenção + scopes)
	penalGroup := r.Group("/penal")
	penalGroup.Use(
		maintenanceMiddleware.Middleware(), // Verifica manutenção
		auth.AuthAPIKey(apikeys),           // Requer API Key válida
		auth.RequireScope(apikeys, "penal"), // ✅ Verifica scope 'penal' ou 'all'
		rateLimiter.Middleware(),           // Aplica rate limiting
		usageLogger.Middleware(),           // Loga uso
	)
	{
		penalGroup.GET("/artigos", penalHandler.ListArtigos)
		penalGroup.GET("/artigos/:codigo", penalHandler.GetArtigo)
		penalGroup.GET("/search", penalHandler.SearchArtigos)
	}

	// Admin endpoints (protegidos por JWT + role SUPER_ADMIN)
	adminHandler := handlers.NewAdminHandler(tenants, apikeys, users, m)
	adminGroup := r.Group("/admin")
	adminGroup.Use(auth.AuthJWT(jwtService), auth.RequireSuperAdmin())
	{
		// Tenants (admin only)
		tenantsHandler := handlers.NewTenantsHandler(tenants, activityLogs, settings)
		adminGroup.GET("/tenants", tenantsHandler.List)
		adminGroup.GET("/tenants/:id", tenantsHandler.Get)
		adminGroup.POST("/tenants", tenantsHandler.Create)
		adminGroup.PUT("/tenants/:id", tenantsHandler.Update)
		adminGroup.DELETE("/tenants/:id", tenantsHandler.Delete)

		// API Keys (admin only)
		apikeysHandler := handlers.NewAPIKeysHandler(apikeys, tenants, activityLogs)
		adminGroup.GET("/apikeys", adminHandler.ListAllAPIKeys)
		adminGroup.POST("/apikeys", apikeysHandler.Create)
		adminGroup.POST("/apikeys/rotate", apikeysHandler.Rotate)
		adminGroup.POST("/apikeys/revoke", apikeysHandler.Revoke)

		// Analytics (admin only)
		adminGroup.GET("/stats", adminHandler.GetStats)
		adminGroup.GET("/usage", adminHandler.GetUsage)
		adminGroup.GET("/analytics", adminHandler.GetAnalytics) // ✅ NOVO: Analytics detalhado com breakdown por API

		// Settings (admin only)
		settingsHandler := handlers.NewSettingsHandler(settings, activityLogs)
		adminGroup.GET("/settings", settingsHandler.Get)
		adminGroup.PUT("/settings", settingsHandler.Update)

		// Playground API Key Management (admin only)
		playgroundAPIKeyHandler := handlers.NewPlaygroundAPIKeyHandler(apikeys, settings, m.DB)
		adminGroup.POST("/playground/apikey/generate", playgroundAPIKeyHandler.GenerateAPIKey)
		adminGroup.POST("/playground/apikey/rotate", playgroundAPIKeyHandler.RotateAPIKey)

		// Cache Management (admin only)
		adminGroup.GET("/cache/cep/stats", cepHandler.GetCacheStats)
		adminGroup.DELETE("/cache/cep", cepHandler.ClearCache)
		adminGroup.GET("/cache/cnpj/stats", cnpjHandler.GetCacheStats)
		adminGroup.DELETE("/cache/cnpj", cnpjHandler.ClearCache)
		adminGroup.GET("/cache/penal/stats", penalHandler.GetCacheStats)

		// Redis Cache Management (admin only)
		redisStatsHandler := handlers.NewRedisStatsHandler(redisClient)
		adminGroup.GET("/cache/redis/stats", redisStatsHandler.GetStats)
		adminGroup.DELETE("/cache/redis", redisStatsHandler.ClearAll)
		adminGroup.DELETE("/cache/redis/cep", redisStatsHandler.ClearCEP)
		adminGroup.DELETE("/cache/redis/cnpj", redisStatsHandler.ClearCNPJ)

		// Activity Logs (admin only)
		activityHandler := handlers.NewActivityHandler(activityLogs)
		adminGroup.GET("/activity", activityHandler.GetRecent)
		adminGroup.GET("/activity/user/:userId", activityHandler.GetByUser)
		adminGroup.GET("/activity/type/:type", activityHandler.GetByType)
		adminGroup.GET("/activity/resource/:type/:id", activityHandler.GetByResource)
	}

	// Tenant endpoints (protegidos por JWT + role TENANT_USER)
	tenantHandler := handlers.NewTenantHandler(apikeys, users, tenants, m)
	meGroup := r.Group("/me")
	meGroup.Use(auth.AuthJWT(jwtService), auth.RequireTenantUser())
	{
		// Minhas API Keys
		meGroup.GET("/apikeys", tenantHandler.ListMyAPIKeys)
		meGroup.POST("/apikeys", tenantHandler.CreateAPIKey)
		meGroup.POST("/apikeys/:id/rotate", tenantHandler.RotateAPIKey)
		meGroup.DELETE("/apikeys/:id", tenantHandler.DeleteAPIKey)

		// Meu uso
		meGroup.GET("/stats", tenantHandler.GetMyStats)   // Métricas rápidas para dashboard
		meGroup.GET("/usage", tenantHandler.GetMyUsage)   // Uso detalhado com gráficos
		meGroup.GET("/config", tenantHandler.GetMyConfig) // Configurações para docs
	}

	return r
}
