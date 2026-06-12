package domain

import (
	"os"
	"time"
)

// SystemSettings representa as configurações globais do sistema
type SystemSettings struct {
	ID string `bson:"_id,omitempty" json:"id,omitempty"`

	// Rate Limiting DEFAULT (para novos tenants)
	DefaultRateLimit RateLimitConfig `bson:"defaultRateLimit" json:"defaultRateLimit"`

	// CORS
	CORS CORSConfig `bson:"cors" json:"cors"`

	// JWT
	JWT JWTConfig `bson:"jwt" json:"jwt"`

	// API Info
	API APIConfig `bson:"api" json:"api"`

	// Contato/Vendas
	Contact ContactConfig `bson:"contact" json:"contact"`

	// Cache
	Cache CacheConfig `bson:"cache" json:"cache"`

	// Playground
	Playground PlaygroundConfig `bson:"playground" json:"playground"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// CORSConfig define a configuração de CORS
type CORSConfig struct {
	Enabled        bool     `bson:"enabled" json:"enabled"`
	AllowedOrigins []string `bson:"allowedOrigins" json:"allowedOrigins"`
}

// JWTConfig define a configuração de JWT
type JWTConfig struct {
	AccessTokenTTL  int `bson:"accessTokenTTL" json:"accessTokenTTL"`   // em segundos
	RefreshTokenTTL int `bson:"refreshTokenTTL" json:"refreshTokenTTL"` // em segundos
}

// APIConfig define informações da API
type APIConfig struct {
	Version     string `bson:"version" json:"version"`
	Environment string `bson:"environment" json:"environment"` // development, production
	Maintenance bool   `bson:"maintenance" json:"maintenance"`
}

// ContactConfig define informações de contato/vendas
type ContactConfig struct {
	WhatsApp string `bson:"whatsapp" json:"whatsapp"` // Formato: 48999616679
	Email    string `bson:"email" json:"email"`
	Phone    string `bson:"phone" json:"phone"`
}

// CacheConfig define a configuração de cache (DEPRECATED: usar CEP e CNPJ individuais)
type CacheConfig struct {
	// ⚠️ DEPRECATED: Mantido para retrocompatibilidade
	Enabled      bool `bson:"enabled,omitempty" json:"enabled,omitempty"`
	CEPTTLDays   int  `bson:"cepTtlDays,omitempty" json:"cepTtlDays,omitempty"`
	CNPJTTLDays  int  `bson:"cnpjTtlDays,omitempty" json:"cnpjTtlDays,omitempty"`
	MaxSizeMB    int  `bson:"maxSizeMb,omitempty" json:"maxSizeMb,omitempty"`
	AutoCleanup  bool `bson:"autoCleanup,omitempty" json:"autoCleanup,omitempty"`
	
	// ✅ NOVO: Controles independentes por serviço
	CEP   ServiceCacheConfig `bson:"cep" json:"cep"`
	CNPJ  ServiceCacheConfig `bson:"cnpj" json:"cnpj"`
	Penal ServiceCacheConfig `bson:"penal" json:"penal"`
}

// ServiceCacheConfig define configuração de cache para um serviço específico
type ServiceCacheConfig struct {
	Enabled     bool `bson:"enabled" json:"enabled"`         // Habilitar cache para este serviço
	TTLDays     int  `bson:"ttlDays" json:"ttlDays"`         // TTL em dias (1-365)
	AutoCleanup bool `bson:"autoCleanup" json:"autoCleanup"` // Limpeza automática via TTL index
}

// PlaygroundConfig define a configuração do playground público
type PlaygroundConfig struct {
	Enabled     bool             `bson:"enabled" json:"enabled"`               // Habilitar/Desabilitar playground
	APIKey      string           `bson:"apiKey" json:"apiKey"`                 // API Key demo (editável)
	RateLimit   RateLimitConfig  `bson:"rateLimit" json:"rateLimit"`           // Rate limit do playground
	AllowedAPIs []string         `bson:"allowedApis" json:"allowedApis"`       // APIs disponíveis ['cep', 'cnpj', 'geo']
}

// GetDefaultSettings retorna as configurações padrão do sistema
func GetDefaultSettings() *SystemSettings {
	// Detectar ambiente da variável ENV (padrão: development)
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("NODE_ENV")
	}
	if env == "" {
		env = "development"
	}
	
	return &SystemSettings{
		DefaultRateLimit: PlanLimits(PlanFree), // Pricing v2: free = 100/dia, 5/min
		CORS: CORSConfig{
			Enabled: true,
			AllowedOrigins: []string{
				"https://core.theretech.com.br",
				"http://localhost:3000",
				"http://localhost:3001",
				"http://localhost:3002",
				"http://localhost:3003",
			},
		},
		JWT: JWTConfig{
			AccessTokenTTL:  900,    // 15 minutos
			RefreshTokenTTL: 604800, // 7 dias
		},
		API: APIConfig{
			Version:     "1.0.0",
			Environment: env, // ← Agora vem da variável de ambiente!
			Maintenance: false,
		},
		Contact: ContactConfig{
			WhatsApp: "48999616679", // ✅ Seu WhatsApp padrão
			Email:    "suporte@theretech.com.br",
			Phone:    "+55 48 99961-6679",
		},
		Cache: CacheConfig{
			// ✅ Controles independentes por serviço
			CEP: ServiceCacheConfig{
				Enabled:     true, // Cache CEP habilitado
				TTLDays:     7,    // 7 dias
				AutoCleanup: true, // Limpeza automática ativa
			},
			CNPJ: ServiceCacheConfig{
				Enabled:     true, // Cache CNPJ habilitado
				TTLDays:     30,   // 30 dias (empresas não mudam frequentemente)
				AutoCleanup: true, // Limpeza automática ativa
			},
			Penal: ServiceCacheConfig{
				Enabled:     true,  // Cache Penal habilitado
				TTLDays:     365,   // 365 dias (dados fixos - permanentes)
				AutoCleanup: false, // Não limpa automaticamente (dados fixos)
			},
		},
		Playground: PlaygroundConfig{
			Enabled: true, // ✅ Playground habilitado por padrão
			APIKey:  "rtc_demo_playground_2024", // Chave demo editável
			RateLimit: RateLimitConfig{
				RequestsPerDay:    100, // 100 requests/dia (agressivo)
				RequestsPerMinute: 10,  // 10 requests/minuto
			},
			AllowedAPIs: []string{"cep", "cnpj", "geo"}, // APIs públicas
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

// UpdateSystemSettingsRequest representa o payload para atualizar configurações
type UpdateSystemSettingsRequest struct {
	DefaultRateLimit *RateLimitConfig   `json:"defaultRateLimit,omitempty"`
	CORS             *CORSConfig        `json:"cors,omitempty"`
	JWT              *JWTConfig         `json:"jwt,omitempty"`
	API              *APIConfig         `json:"api,omitempty"`
	Contact          *ContactConfig     `json:"contact,omitempty"`
	Cache            *CacheConfig       `json:"cache,omitempty"`
	Playground       *PlaygroundConfig  `json:"playground,omitempty"`
}
