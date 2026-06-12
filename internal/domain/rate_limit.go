package domain

import "time"

// RateLimit representa um registro de rate limiting por API Key
type RateLimit struct {
	APIKey    string    `bson:"apiKey" json:"apiKey"`
	Date      string    `bson:"date" json:"date"`           // YYYY-MM-DD
	Count     int64     `bson:"count" json:"count"`         // Requests neste dia
	LastReset time.Time `bson:"lastReset" json:"lastReset"` // Último reset
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// RateLimitConfig configuração de limites
type RateLimitConfig struct {
	RequestsPerDay    int64 // Limite diário
	RequestsPerMinute int64 // Limite por minuto
}

// GetDefaultRateLimit retorna limites padrão (plano free — Pricing v2)
func GetDefaultRateLimit() RateLimitConfig {
	return PlanLimits(PlanFree)
}
