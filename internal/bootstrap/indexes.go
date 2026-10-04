package bootstrap

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Helper function for string contains
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func EnsureIndexes(db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// users: email único
	_, err := db.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// refresh_tokens: TTL em expiresAt
	_, err = db.Collection("refresh_tokens").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// api_keys: TTL
	_, err = db.Collection("api_keys").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// rate_limits: índice por API key
	_, err = db.Collection("rate_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "apiKey", Value: 1}},
	})
	if err != nil {
		return err
	}

	// rate_limits: TTL em resetAt
	_, err = db.Collection("rate_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "resetAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// playground_rate_limits: índice composto para segurança
	_, err = db.Collection("playground_rate_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "ipAddress", Value: 1},
			{Key: "apiKey", Value: 1},
			{Key: "date", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// playground_rate_limits: índice por IP para consultas rápidas
	_, err = db.Collection("playground_rate_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "ipAddress", Value: 1},
			{Key: "date", Value: 1},
		},
	})
	if err != nil {
		return err
	}

	// playground_rate_limits: TTL para limpeza automática
	_, err = db.Collection("playground_rate_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(7 * 24 * 60 * 60), // 7 dias
	})
	if err != nil {
		return err
	}

	// playground_global_limits: índice composto
	_, err = db.Collection("playground_global_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "apiKey", Value: 1},
			{Key: "date", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// playground_global_limits: TTL para limpeza automática
	_, err = db.Collection("playground_global_limits").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(7 * 24 * 60 * 60), // 7 dias
	})
	if err != nil {
		return err
	}

	return nil
}

// CreateIndexes cria todos os índices necessários (idempotente)
// ✅ Executa automaticamente ao iniciar a aplicação
// ✅ Idempotente: pode rodar múltiplas vezes sem erros
// ✅ Funciona em dev e produção igualmente
func CreateIndexes(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔧 Criando/verificando índices do MongoDB...")

	// Helper para criar índice com tratamento de erro idempotente
	createIndex := func(collName string, model mongo.IndexModel, description string) error {
		_, err := db.Collection(collName).Indexes().CreateOne(ctx, model)
		if err != nil {
			// Ignorar erro se índice já existe
			if mongo.IsDuplicateKeyError(err) ||
				(err.Error() != "" && (contains(err.Error(), "already exists") ||
					contains(err.Error(), "IndexOptionsConflict"))) {
				log.Debug().Str("collection", collName).Str("index", description).Msg("Índice já existe, pulando...")
				return nil
			}
			log.Error().Err(err).Str("collection", collName).Str("index", description).Msg("Erro ao criar índice")
			return err
		}
		log.Info().Str("collection", collName).Str("index", description).Msg("✅ Índice criado/verificado")
		return nil
	}

	// Estados: índice único por ID e por sigla
	if err := createIndex("estados", mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "id_unique"); err != nil {
		return err
	}

	if err := createIndex("estados", mongo.IndexModel{
		Keys:    bson.D{{Key: "sigla", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "sigla_unique"); err != nil {
		return err
	}

	// Municípios: índice único por ID
	if err := createIndex("municipios", mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "id_unique"); err != nil {
		return err
	}

	// Municípios: índice para busca por UF
	if err := createIndex("municipios", mongo.IndexModel{
		Keys: bson.D{{Key: "microrregiao.mesorregiao.UF.sigla", Value: 1}},
	}, "uf_sigla"); err != nil {
		return err
	}

	// Municípios: índice para busca por nome
	if err := createIndex("municipios", mongo.IndexModel{
		Keys: bson.D{{Key: "nome", Value: 1}},
	}, "nome"); err != nil {
		return err
	}

	// ✅ PERFORMANCE: Índice único para CEP cache (hot path)
	if err := createIndex("cep_cache", mongo.IndexModel{
		Keys:    bson.D{{Key: "cep", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "cep_unique"); err != nil {
		return err
	}

	// ✅ PERFORMANCE: Índice único para CNPJ cache (hot path)
	if err := createIndex("cnpj_cache", mongo.IndexModel{
		Keys:    bson.D{{Key: "cnpj", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "cnpj_unique"); err != nil {
		return err
	}

	// ✅ PERFORMANCE: Índices para penal_artigos (dados fixos, cache permanente)
	// Remover índices antigos que podem causar conflito (migração)
	coll := db.Collection("penal_artigos")
	indexes, err := coll.Indexes().List(ctx)
	indicesRemovidos := []string{}
	if err == nil && indexes != nil {
		for indexes.Next(ctx) {
			var idx bson.M
			if indexes.Decode(&idx) == nil {
				name, _ := idx["name"].(string)
				key, _ := idx["key"].(bson.M)
				unique, _ := idx["unique"].(bool)

				// Remover índice único em "codigo" (seja codigo_unique, codigo_1, etc)
				if name != "" && unique {
					if key != nil {
						if _, hasCodigo := key["codigo"]; hasCodigo {
							// É um índice único em codigo - remover
							if name != "idunico_unique" { // Não remover o índice correto
								log.Info().Str("index", name).Msg("[index] Removendo índice único antigo em codigo...")
								_, err := coll.Indexes().DropOne(ctx, name)
								if err == nil {
									indicesRemovidos = append(indicesRemovidos, name)
									log.Info().Str("index", name).Msg("[index] ✅ Índice antigo removido")
								}
							}
						}
					}
				}
			}
		}
		indexes.Close(ctx)
	}
	if len(indicesRemovidos) > 0 {
		log.Info().Strs("indices", indicesRemovidos).Msg("[index] Removidos índices antigos que causavam conflito")
	}

	// Índice único em idUnico (que combina legislação + código)
	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys:    bson.D{{Key: "idUnico", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, "idunico_unique"); err != nil {
		return err
	}

	// Índice não-único em codigo (para busca rápida, mas permite duplicatas entre legislações)
	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "codigo", Value: 1}},
	}, "codigo"); err != nil {
		return err
	}

	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "artigo", Value: 1}, {Key: "paragrafo", Value: 1}},
	}, "artigo_paragrafo"); err != nil {
		return err
	}

	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "busca", Value: 1}},
	}, "busca_text"); err != nil {
		return err
	}

	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "tipo", Value: 1}},
	}, "tipo"); err != nil {
		return err
	}

	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "legislacao", Value: 1}},
	}, "legislacao"); err != nil {
		return err
	}

	// Penal v2: ordenação global estável (CP primeiro, depois leis especiais)
	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "ordem", Value: 1}},
	}, "ordem"); err != nil {
		return err
	}

	// Penal v2: listagem por legislação já ordenada (filtro + sort cobertos pelo índice)
	if err := createIndex("penal_artigos", mongo.IndexModel{
		Keys: bson.D{{Key: "legislacao", Value: 1}, {Key: "ordem", Value: 1}},
	}, "legislacao_ordem"); err != nil {
		return err
	}

	// ✅ PERFORMANCE: Índice para tenant_id (hot path - rate limiting)
	if err := createIndex("rate_limits", mongo.IndexModel{
		Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "resetAt", Value: 1}},
	}, "tenant_reset"); err != nil {
		return err
	}

	// ✅ PERFORMANCE: Índice composto para rate_limits_minute
	if err := createIndex("rate_limits_minute", mongo.IndexModel{
		Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "resetAt", Value: 1}},
	}, "tenant_reset"); err != nil {
		return err
	}

	// 🔒 SEGURANÇA: Índice composto para playground_rate_limits (IP + API Key + Data)
	if err := createIndex("playground_rate_limits", mongo.IndexModel{
		Keys: bson.D{
			{Key: "ipAddress", Value: 1},
			{Key: "apiKey", Value: 1},
			{Key: "date", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}, "ip_apikey_date_unique"); err != nil {
		return err
	}

	// 🔒 SEGURANÇA: Índice por IP para consultas rápidas
	if err := createIndex("playground_rate_limits", mongo.IndexModel{
		Keys: bson.D{
			{Key: "ipAddress", Value: 1},
			{Key: "date", Value: 1},
		},
	}, "ip_date"); err != nil {
		return err
	}

	// 🔒 SEGURANÇA: TTL para limpeza automática (7 dias)
	if err := createIndex("playground_rate_limits", mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(7 * 24 * 60 * 60),
	}, "ttl_7days"); err != nil {
		return err
	}

	// 🔒 SEGURANÇA: Índice composto para playground_global_limits
	if err := createIndex("playground_global_limits", mongo.IndexModel{
		Keys: bson.D{
			{Key: "apiKey", Value: 1},
			{Key: "date", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}, "apikey_date_unique"); err != nil {
		return err
	}

	// 🔒 SEGURANÇA: TTL para limpeza automática (7 dias)
	if err := createIndex("playground_global_limits", mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(7 * 24 * 60 * 60),
	}, "ttl_7days"); err != nil {
		return err
	}

	log.Info().Msg("✅ Todos os índices foram criados/verificados com sucesso (incluindo segurança multi-camada)!")
	return nil
}

// MigrateSettings adiciona campos faltantes nas configurações
func MigrateSettings(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("Migrando configurações...")

	// Verificar se settings tem o campo contact
	var settings bson.M
	err := db.Collection("system_settings").FindOne(ctx, bson.M{
		"_id": "system-settings-singleton",
	}).Decode(&settings)

	if err == nil {
		// Settings existe, verificar se tem contact
		if _, hasContact := settings["contact"]; !hasContact {
			log.Info().Msg("Adicionando campo contact nas configurações...")

			_, err = db.Collection("system_settings").UpdateOne(
				ctx,
				bson.M{"_id": "system-settings-singleton"},
				bson.M{
					"$set": bson.M{
						"contact": bson.M{
							"whatsapp": "48999616679",
							"email":    "suporte@theretech.com.br",
							"phone":    "+55 48 99961-6679",
						},
					},
				},
			)

			if err != nil {
				log.Error().Err(err).Msg("Erro ao migrar campo contact")
				return err
			}

			log.Info().Msg("Campo contact adicionado com sucesso!")
		}

		// Verificar se settings tem o campo cache
		if _, hasCache := settings["cache"]; !hasCache {
			log.Info().Msg("Adicionando campo cache nas configurações...")

			_, err = db.Collection("system_settings").UpdateOne(
				ctx,
				bson.M{"_id": "system-settings-singleton"},
				bson.M{
					"$set": bson.M{
						"cache": bson.M{
							"enabled":     true,
							"cepTtlDays":  7,
							"cnpjTtlDays": 30, // ✅ CNPJ: 30 dias
							"maxSizeMb":   100,
							"autoCleanup": true,
						},
					},
				},
			)

			if err != nil {
				log.Error().Err(err).Msg("Erro ao migrar campo cache")
				return err
			}

			log.Info().Msg("Campo cache adicionado com sucesso!")
		}
	}

	log.Info().Msg("Migração de configurações concluída")
	return nil
}
