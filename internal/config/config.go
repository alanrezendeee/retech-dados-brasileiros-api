package config

import (
	"fmt"
	"log"
	"os"
	"time"
)

type Config struct {
	Env            string
	HTTPPort       string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MongoURI       string
	MongoDB        string
	EnableCORS     bool
	CORSOrigins    []string
	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTAccessTTL     time.Duration
	JWTRefreshTTL    time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ValidateCoreConfig valida ENVs obrigatórias de infraestrutura.
// Chame antes de qualquer outra coisa no main — falha rápido se algo falta.
func ValidateCoreConfig() {
	const devAccessSecret = "dev-access-secret-change-in-production"
	const devRefreshSecret = "dev-refresh-secret-change-in-production"

	missing := []string{}

	if os.Getenv("MONGO_URI") == "" {
		missing = append(missing, "MONGO_URI")
	}
	if s := os.Getenv("JWT_ACCESS_SECRET"); s == "" || s == devAccessSecret {
		missing = append(missing, "JWT_ACCESS_SECRET (não pode usar o valor padrão de dev)")
	}
	if s := os.Getenv("JWT_REFRESH_SECRET"); s == "" || s == devRefreshSecret {
		missing = append(missing, "JWT_REFRESH_SECRET (não pode usar o valor padrão de dev)")
	}
	if os.Getenv("REDIS_URL") == "" {
		missing = append(missing, "REDIS_URL")
	}
	if os.Getenv("CEPDB_URL") == "" {
		missing = append(missing, "CEPDB_URL")
	}

	if len(missing) > 0 {
		fmt.Printf("\n🔴 ERRO DE CONFIGURAÇÃO: Variáveis obrigatórias ausentes ou inválidas:\n")
		for _, env := range missing {
			fmt.Printf("   - %s\n", env)
		}
		fmt.Printf("\nConfigure as variáveis e reinicie. Ver env.example.\n\n")
		panic("Configuração de infraestrutura incompleta!")
	}

	fmt.Printf("✅ [CONFIG] Configuração de infraestrutura validada\n")
}

func Load() *Config {
	c := &Config{
		Env:              getenv("ENV", "development"),
		HTTPPort:         getenv("PORT", "8080"),
		MongoURI:         os.Getenv("MONGO_URI"),
		MongoDB:          getenv("MONGO_DB", "retech_core"),
		EnableCORS:       getenv("CORS_ENABLE", "true") == "true",
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     15 * time.Second,
		IdleTimeout:      60 * time.Second,
		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		JWTAccessTTL:     15 * time.Minute,
		JWTRefreshTTL:    7 * 24 * time.Hour,
	}
	log.Printf("[config] ENV=%s PORT=%s MONGO_URI=%s DB=%s",
		c.Env, c.HTTPPort, c.MongoURI, c.MongoDB)
	return c
}

