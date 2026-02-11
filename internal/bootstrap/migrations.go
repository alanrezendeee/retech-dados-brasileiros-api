package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/theretech/retech-core/internal/domain"
	"github.com/theretech/retech-core/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Migration representa uma migração/seed
type Migration struct {
	Version     string
	Description string
	Apply       func(ctx context.Context, db *mongo.Database, log zerolog.Logger) error
}

// MigrationRecord registra migrations executadas
type MigrationRecord struct {
	Version     string    `bson:"version"`
	Description string    `bson:"description"`
	AppliedAt   time.Time `bson:"appliedAt"`
}

// MigrationManager gerencia as migrations
type MigrationManager struct {
	db         *mongo.Database
	log        zerolog.Logger
	migrations []Migration
}

// NewMigrationManager cria um novo gerenciador de migrations
func NewMigrationManager(db *mongo.Database, log zerolog.Logger) *MigrationManager {
	return &MigrationManager{
		db:  db,
		log: log,
		migrations: []Migration{
			{
				Version:     "001_seed_estados",
				Description: "Popular estados brasileiros",
				Apply:       seedEstados,
			},
			{
				Version:     "002_seed_municipios",
				Description: "Popular municípios brasileiros",
				Apply:       seedMunicipios,
			},
			{
				Version:     "003_seed_penal",
				Description: "Popular artigos penais brasileiros",
				Apply:       seedPenal,
			},
			{
				Version:     "004_update_penal",
				Description: "Atualizar artigos penais (inclui importunação sexual - 215, 215-A, 216-A)",
				Apply:       seedPenal, // Reutiliza a mesma função que faz upsert
			},
			{
				Version:     "005_fix_article_157",
				Description: "Corrigir estrutura completa do artigo 157 do CP (7 dispositivos)",
				Apply:       migration005FixArticle157,
			},
			{
				Version:     "006_add_article_157_missing",
				Description: "Adicionar artigos 157 que faltaram na migration 005",
				Apply:       migration006AddArticle157Missing,
			},
			{
				Version:     "007_add_article_331",
				Description: "Adicionar artigo 331 (Desacato) do CP",
				Apply:       migration007AddArticle331,
			},
			{
				Version:     "008_fix_articles_47_337",
				Description: "Corrigir descrições dos artigos 47 (LCP) e 337 (CP)",
				Apply:       migration008FixArticles47And337,
			},
			{
				Version:     "009_add_articles_12_211_307_329_349",
				Description: "Adicionar artigos 12 (DES), 211, 307, 329 e 349 (CP)",
				Apply:       migration009AddArticles12_211_307_329_349,
			},
		},
	}
}

// Run executa as migrations pendentes
func (m *MigrationManager) Run(ctx context.Context) error {
	coll := m.db.Collection("migrations")

	for _, migration := range m.migrations {
		// Verifica se já foi aplicada
		count, err := coll.CountDocuments(ctx, bson.M{"version": migration.Version})
		if err != nil {
			return fmt.Errorf("erro ao verificar migration %s: %w", migration.Version, err)
		}

		if count > 0 {
			m.log.Info().Msgf("[migration] %s já aplicada, pulando", migration.Version)
			continue
		}

		// Aplica a migration
		m.log.Info().Msgf("[migration] Aplicando %s: %s", migration.Version, migration.Description)
		start := time.Now()

		if err := migration.Apply(ctx, m.db, m.log); err != nil {
			return fmt.Errorf("erro ao aplicar migration %s: %w", migration.Version, err)
		}

		// Registra como aplicada
		record := MigrationRecord{
			Version:     migration.Version,
			Description: migration.Description,
			AppliedAt:   time.Now(),
		}
		if _, err := coll.InsertOne(ctx, record); err != nil {
			return fmt.Errorf("erro ao registrar migration %s: %w", migration.Version, err)
		}

		m.log.Info().Msgf("[migration] %s aplicada com sucesso em %v", migration.Version, time.Since(start))
	}

	return nil
}

// seedEstados popula os estados
func seedEstados(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	repo := storage.NewEstadosRepo(db)

	// Verifica se já existem dados
	count, err := repo.Count(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		log.Info().Msgf("[seed] Estados já populados (%d registros), pulando", count)
		return nil
	}

	// Procura o arquivo estados.json
	seedFile := findSeedFile("estados.json")
	if seedFile == "" {
		return fmt.Errorf("arquivo estados.json não encontrado")
	}

	log.Info().Msgf("[seed] Carregando estados de: %s", seedFile)

	// Lê o arquivo
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo estados.json: %w", err)
	}

	var estados []domain.Estado
	if err := json.Unmarshal(data, &estados); err != nil {
		return fmt.Errorf("erro ao fazer parse de estados.json: %w", err)
	}

	// Insere no banco
	if err := repo.InsertMany(ctx, estados); err != nil {
		return fmt.Errorf("erro ao inserir estados: %w", err)
	}

	log.Info().Msgf("[seed] %d estados inseridos com sucesso", len(estados))
	return nil
}

// seedMunicipios popula os municípios
func seedMunicipios(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	repo := storage.NewMunicipiosRepo(db)

	// Verifica se já existem dados
	count, err := repo.Count(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		log.Info().Msgf("[seed] Municípios já populados (%d registros), pulando", count)
		return nil
	}

	// Procura o arquivo municipios.json
	seedFile := findSeedFile("municipios.json")
	if seedFile == "" {
		return fmt.Errorf("arquivo municipios.json não encontrado")
	}

	log.Info().Msgf("[seed] Carregando municípios de: %s", seedFile)

	// Lê o arquivo
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo municipios.json: %w", err)
	}

	var municipios []domain.Municipio
	if err := json.Unmarshal(data, &municipios); err != nil {
		return fmt.Errorf("erro ao fazer parse de municipios.json: %w", err)
	}

	log.Info().Msgf("[seed] Inserindo %d municípios (isso pode demorar)...", len(municipios))

	// Insere no banco em lotes
	if err := repo.InsertMany(ctx, municipios); err != nil {
		return fmt.Errorf("erro ao inserir municípios: %w", err)
	}

	log.Info().Msgf("[seed] %d municípios inseridos com sucesso", len(municipios))
	return nil
}

// findSeedFile procura o arquivo de seed em diversos locais
func findSeedFile(filename string) string {
	// Possíveis localizações (em ordem de prioridade)
	locations := []string{
		// 1. Diretório seeds (padrão - funciona local e Docker)
		filepath.Join("seeds", filename),
		// 2. Diretório /app/seeds (Docker com volume montado)
		filepath.Join("/app", "seeds", filename),
		// 3. Diretório atual
		filename,
		// 4. Downloads do usuário (desenvolvimento local)
		filepath.Join(os.Getenv("HOME"), "Downloads", filename),
		// 5. Diretório data
		filepath.Join("data", filename),
		// 6. Caminho absoluto no workdir
		filepath.Join(".", "seeds", filename),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// seedPenal popula os artigos penais
// Estratégia inteligente: usa upsert baseado em idUnico para adicionar/atualizar apenas o necessário
func seedPenal(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	collection := db.Collection("penal_artigos")

	// Procura o arquivo penal.json
	seedFile := findSeedFile("penal.json")
	if seedFile == "" {
		return fmt.Errorf("arquivo penal.json não encontrado")
	}

	log.Info().Msgf("[seed] Carregando artigos penais de: %s", seedFile)

	// Lê o arquivo
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo penal.json: %w", err)
	}

	var artigos []domain.ArtigoPenal
	if err := json.Unmarshal(data, &artigos); err != nil {
		return fmt.Errorf("erro ao fazer parse de penal.json: %w", err)
	}

	log.Info().Msgf("[seed] Arquivo penal.json contém %d artigos", len(artigos))

	// Verificar quantos já existem no banco
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}

	log.Info().Msgf("[seed] Banco de dados contém %d artigos", count)

	// Mapeamento de legislações para códigos curtos (para idUnico)
	legislacaoCodes := map[string]string{
		"CP":              "CP",
		"LCP":             "LCP",
		"Lei 11.343/2006": "DRG", // Drogas
		"ECA":             "ECA",
		"CTB":             "CTB",
		"Lei 9.605/98":    "AMB", // Ambiente
		"CDC":             "CDC",
		"Lei 9.613/98":    "LVD", // Lavagem
	}

	// Estratégia: Upsert baseado em idUnico
	// - Se o artigo já existe (por idUnico) → atualiza
	// - Se o artigo não existe → insere
	// Isso permite adicionar novos artigos sem limpar tudo
	now := time.Now()
	inserted := 0
	updated := 0
	
	for _, artigo := range artigos {
		// Preparar artigo com timestamps e campos normalizados
		artigo.UpdatedAt = now
		if artigo.CreatedAt.IsZero() {
			artigo.CreatedAt = now
		}
		
		// Normalizar campo busca (lowercase)
		artigo.Busca = strings.ToLower(artigo.Descricao + " " + artigo.TextoCompleto + " " + artigo.CodigoFormatado)
		
		// Gerar idUnico com código curto (se não existir)
		if artigo.IdUnico == "" {
			legCode := legislacaoCodes[artigo.Legislacao]
			if legCode == "" {
				legCode = artigo.Legislacao
			}
			artigo.IdUnico = fmt.Sprintf("%s:%s", legCode, artigo.Codigo)
		} else {
			// Atualizar idUnico para código curto se necessário
			parts := strings.Split(artigo.IdUnico, ":")
			if len(parts) == 2 {
				legOriginal := parts[0]
				codigoPart := parts[1]
				legCode := legislacaoCodes[legOriginal]
				if legCode != "" && legCode != legOriginal {
					artigo.IdUnico = fmt.Sprintf("%s:%s", legCode, codigoPart)
				}
			}
		}
		
		// Gerar hashConteudo se não existir
		if artigo.HashConteudo == "" {
			conteudoHash := fmt.Sprintf("%s:%s:%s", artigo.Legislacao, artigo.Codigo, artigo.TextoCompleto)
			hash := sha256.Sum256([]byte(conteudoHash))
			artigo.HashConteudo = hex.EncodeToString(hash[:])
		}
		
		// Validar idUnico antes de fazer upsert
		if artigo.IdUnico == "" {
			log.Warn().Msgf("[seed] Artigo sem idUnico ignorado: código=%s, legislação=%s", artigo.Codigo, artigo.Legislacao)
			continue
		}
		
		// Upsert: inserir ou atualizar baseado em idUnico
		filter := bson.M{"idUnico": artigo.IdUnico}
		update := bson.M{
			"$set": bson.M{
				"codigo":          artigo.Codigo,
				"artigo":          artigo.Artigo,
				"paragrafo":       artigo.Paragrafo,
				"inciso":          artigo.Inciso,
				"alinea":          artigo.Alinea,
				"descricao":       artigo.Descricao,
				"textoCompleto":   artigo.TextoCompleto,
				"tipo":            artigo.Tipo,
				"legislacao":      artigo.Legislacao,
				"legislacaoNome":  artigo.LegislacaoNome,
				"penaMin":         artigo.PenaMin,
				"penaMax":         artigo.PenaMax,
				"codigoFormatado": artigo.CodigoFormatado,
				"busca":           artigo.Busca,
				"fonte":           artigo.Fonte,
				"dataAtualizacao": artigo.DataAtualizacao,
				"hashConteudo":    artigo.HashConteudo,
				"idUnico":         artigo.IdUnico,
				"updatedAt":       artigo.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"createdAt": artigo.CreatedAt,
			},
		}
		
		opts := options.Update().SetUpsert(true)
		result, err := collection.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			// Log detalhado do erro
			log.Error().Err(err).
				Str("idUnico", artigo.IdUnico).
				Str("codigo", artigo.Codigo).
				Str("legislacao", artigo.Legislacao).
				Msg("[seed] Erro ao fazer upsert do artigo")
			
			// Se for erro de índice único, tentar remover índices problemáticos e tentar novamente
			if strings.Contains(err.Error(), "E11000") || strings.Contains(err.Error(), "duplicate key") {
				log.Warn().Msgf("[seed] Erro de duplicata detectado para %s. Verificando e removendo índices problemáticos...", artigo.IdUnico)
				
				// Listar todos os índices e remover qualquer índice único em "codigo"
				indexes, idxErr := collection.Indexes().List(ctx)
				if idxErr == nil && indexes != nil {
					for indexes.Next(ctx) {
						var idx bson.M
						if indexes.Decode(&idx) == nil {
							name, _ := idx["name"].(string)
							key, _ := idx["key"].(bson.M)
							unique, _ := idx["unique"].(bool)
							
							// Remover índice único em "codigo" (qualquer nome: codigo_unique, codigo_1, etc)
							if unique && key != nil {
								if _, hasCodigo := key["codigo"]; hasCodigo {
									if name != "idunico_unique" { // Não remover o índice correto
										log.Info().Str("index", name).Msgf("[seed] Removendo índice único problemático %s", name)
										_, _ = collection.Indexes().DropOne(ctx, name)
									}
								}
							}
						}
					}
					indexes.Close(ctx)
				}
				
				// Tentar novamente após remover índices problemáticos
				result, err = collection.UpdateOne(ctx, filter, update, opts)
				if err != nil {
					log.Error().Err(err).Msgf("[seed] Erro persistente ao inserir artigo %s mesmo após remover índices", artigo.IdUnico)
					continue
				}
				log.Info().Msgf("[seed] ✅ Artigo %s inserido após correção de índices", artigo.IdUnico)
			} else {
				continue
			}
		}
		
		if result.UpsertedCount > 0 {
			inserted++
		} else if result.ModifiedCount > 0 {
			updated++
		}
	}

	log.Info().Msgf("[seed] Processados %d artigos: %d inseridos, %d atualizados", len(artigos), inserted, updated)
	
	// Verificar contagem final e comparar com esperado
	finalCount, err := collection.CountDocuments(ctx, bson.M{})
	if err == nil {
		expectedCount := int64(len(artigos))
		log.Info().Msgf("[seed] Total de artigos no banco após seed: %d", finalCount)
		if finalCount < expectedCount {
			log.Warn().Msgf("[seed] ⚠️  ATENÇÃO: Esperados %d artigos, mas apenas %d foram inseridos/atualizados. Verificando artigos faltantes...", expectedCount, finalCount)
			
			// Buscar todos os idUnicos que estão no banco
			cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"idUnico": 1}))
			idunicosNoBanco := make(map[string]bool)
			if err == nil {
				defer cursor.Close(ctx)
				var docs []bson.M
				if err := cursor.All(ctx, &docs); err == nil {
					for _, doc := range docs {
						if id, ok := doc["idUnico"].(string); ok {
							idunicosNoBanco[id] = true
						}
					}
				}
			}
			
			// Identificar artigos do JSON que não estão no banco
			faltantes := []string{}
			for _, artigo := range artigos {
				if !idunicosNoBanco[artigo.IdUnico] {
					faltantes = append(faltantes, artigo.IdUnico)
				}
			}
			
			if len(faltantes) > 0 {
				log.Warn().Msgf("[seed] 📋 %d artigos faltantes identificados: %v", len(faltantes), faltantes)
				log.Info().Msg("[seed] Tentando inserir artigos faltantes novamente...")
				
				// Tentar inserir os faltantes novamente
				faltantesInseridos := 0
				for _, idUnico := range faltantes {
					for _, artigo := range artigos {
						if artigo.IdUnico == idUnico {
							// Preparar artigo novamente
							artigo.UpdatedAt = time.Now()
							if artigo.CreatedAt.IsZero() {
								artigo.CreatedAt = time.Now()
							}
							artigo.Busca = strings.ToLower(artigo.Descricao + " " + artigo.TextoCompleto + " " + artigo.CodigoFormatado)
							
							filter := bson.M{"idUnico": artigo.IdUnico}
							update := bson.M{
								"$set": bson.M{
									"codigo":          artigo.Codigo,
									"artigo":          artigo.Artigo,
									"paragrafo":       artigo.Paragrafo,
									"inciso":          artigo.Inciso,
									"alinea":          artigo.Alinea,
									"descricao":       artigo.Descricao,
									"textoCompleto":   artigo.TextoCompleto,
									"tipo":            artigo.Tipo,
									"legislacao":      artigo.Legislacao,
									"legislacaoNome":  artigo.LegislacaoNome,
									"penaMin":         artigo.PenaMin,
									"penaMax":         artigo.PenaMax,
									"codigoFormatado": artigo.CodigoFormatado,
									"busca":           artigo.Busca,
									"fonte":           artigo.Fonte,
									"dataAtualizacao": artigo.DataAtualizacao,
									"hashConteudo":    artigo.HashConteudo,
									"idUnico":         artigo.IdUnico,
									"updatedAt":       artigo.UpdatedAt,
								},
								"$setOnInsert": bson.M{
									"createdAt": artigo.CreatedAt,
								},
							}
							
							opts := options.Update().SetUpsert(true)
							result, err := collection.UpdateOne(ctx, filter, update, opts)
							if err == nil && result.UpsertedCount > 0 {
								faltantesInseridos++
								log.Info().Msgf("[seed] ✅ Artigo faltante %s inserido com sucesso", artigo.IdUnico)
							} else if err != nil {
								log.Error().Err(err).Msgf("[seed] ❌ Erro ao inserir artigo faltante %s", artigo.IdUnico)
							}
							break
						}
					}
				}
				
				if faltantesInseridos > 0 {
					log.Info().Msgf("[seed] ✅ %d artigos faltantes foram inseridos com sucesso", faltantesInseridos)
				}
			}
			
			// Verificar contagem final novamente
			finalCount, _ = collection.CountDocuments(ctx, bson.M{})
			if finalCount < expectedCount {
				log.Warn().Msgf("[seed] ⚠️  Ainda faltam %d artigos. Verifique os logs de erro acima.", expectedCount-finalCount)
			} else {
				log.Info().Msgf("[seed] ✅ Todos os %d artigos foram inseridos com sucesso!", finalCount)
			}
		} else {
			log.Info().Msgf("[seed] ✅ Todos os %d artigos foram processados com sucesso", finalCount)
		}
	}
	
	return nil
}

// migration005FixArticle157 corrige a estrutura completa do artigo 157 do CP
func migration005FixArticle157(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔄 Executando migration 005: Correção completa do artigo 157 do CP...")
	
	// 1. Deletar o CP:157.1 antigo (estrutura incorreta)
	coll := db.Collection("penal_artigos")
	
	// Primeiro, fazer backup do artigo atual
	var oldArticle bson.M
	err := coll.FindOne(ctx, bson.M{"idUnico": "CP:157.1"}).Decode(&oldArticle)
	if err == nil {
		log.Info().Msgf("📋 Backup do CP:157.1 atual: %v", oldArticle["descricao"])
	}
	
	// Deletar o antigo CP:157.1
	result, err := coll.DeleteOne(ctx, bson.M{"idUnico": "CP:157.1"})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao deletar CP:157.1 antigo: %v", err)
	} else if result.DeletedCount > 0 {
		log.Info().Msg("✅ CP:157.1 antigo deletado com sucesso")
	}
	
	// 2. Carregar dados atualizados do penal.json
	seedFile := findSeedFile("penal.json")
	if seedFile == "" {
		return fmt.Errorf("arquivo penal.json não encontrado")
	}
	
	log.Info().Msgf("📁 Carregando artigos de: %s", seedFile)
	
	data, err := os.ReadFile(seedFile)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo penal.json: %w", err)
	}

	var artigos []domain.ArtigoPenal
	if err := json.Unmarshal(data, &artigos); err != nil {
		return err
	}
	
	log.Info().Msgf("📊 Total de artigos no JSON: %d", len(artigos))

	// 3. Filtrar apenas os artigos do 157 que precisam ser atualizados/inseridos
	art157Updates := []string{
		"CP:157",     // Atualizar caso tenha mudado
		"CP:157.1",   // Novo - Roubo impróprio
		"CP:157.2",   // Atualizar - Causas de aumento
		"CP:157.2-A", // Novo - Aumento 2/3
		"CP:157.2-B", // Novo - Aumento em dobro
		"CP:157.3.I", // Novo - Lesão grave
		"CP:157.3.II",// Novo - Latrocínio
	}

	updateCount := 0
	insertCount := 0

	for _, artigo := range artigos {
		// Log para debug
		if strings.HasPrefix(artigo.Codigo, "157") {
			log.Info().Msgf("🔍 Verificando artigo: Codigo=%s, IdUnico=%s", artigo.Codigo, artigo.IdUnico)
		}
		
		// Processar apenas artigos do 157
		found := false
		for _, id := range art157Updates {
			if artigo.IdUnico == id {
				found = true
				log.Info().Msgf("✅ Artigo encontrado para processar: %s", id)
				break
			}
		}
		if !found {
			continue
		}

		// Adicionar campos calculados
		artigo.Busca = artigo.Codigo + " " + artigo.Descricao + " " + artigo.TextoCompleto
		artigo.CreatedAt = time.Now()
		artigo.UpdatedAt = time.Now()

		// Upsert do artigo
		opts := options.Update().SetUpsert(true)
		filter := bson.M{"idUnico": artigo.IdUnico}
		update := bson.M{
			"$set": artigo,
			"$setOnInsert": bson.M{
				"createdAt": time.Now(),
			},
		}

		result, err := coll.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			log.Error().Msgf("❌ Erro ao processar %s: %v", artigo.IdUnico, err)
			continue
		}

		if result.UpsertedCount > 0 {
			insertCount++
			log.Info().Msgf("✅ Inserido: %s - %s", artigo.IdUnico, artigo.Descricao)
		} else if result.ModifiedCount > 0 {
			updateCount++
			log.Info().Msgf("📝 Atualizado: %s - %s", artigo.IdUnico, artigo.Descricao)
		}
	}

	// 4. Verificar total de artigos
	totalCount, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar artigos: %v", err)
	} else {
		log.Info().Msgf("📊 Total de artigos no banco: %d", totalCount)
		if totalCount < 116 {
			log.Warn().Msgf("⚠️ ATENÇÃO: Esperado 116 artigos, mas temos apenas %d", totalCount)
		}
	}

	// 5. Verificar especificamente os artigos do 157
	count157, err := coll.CountDocuments(ctx, bson.M{"artigo": 157})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar artigos 157: %v", err)
	} else {
		log.Info().Msgf("📊 Total de variações do artigo 157: %d (esperado: 7)", count157)
	}

	log.Info().Msgf("✅ Migration 005 concluída! Inseridos: %d, Atualizados: %d", insertCount, updateCount)
	return nil
}

// migration006AddArticle157Missing adiciona os artigos do 157 que faltaram
func migration006AddArticle157Missing(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔄 Executando migration 006: Adicionar artigos 157 faltantes...")
	
	// Usar a função seedPenal que já tem toda a lógica correta
	// incluindo o mapeamento correto do campo IdUnico
	err := seedPenal(ctx, db, log)
	if err != nil {
		return fmt.Errorf("erro ao executar seedPenal: %w", err)
	}
	
	// Verificar especificamente os artigos do 157
	coll := db.Collection("penal_artigos")
	count157, err := coll.CountDocuments(ctx, bson.M{"artigo": 157})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar artigos 157: %v", err)
	} else {
		log.Info().Msgf("📊 Total de variações do artigo 157: %d (esperado: 7)", count157)
		if count157 >= 7 {
			log.Info().Msg("✅ Todos os artigos 157 foram adicionados com sucesso!")
		}
	}
	
	// Verificar total geral
	totalCount, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar total: %v", err)
	} else {
		log.Info().Msgf("📊 Total de artigos no banco: %d (esperado: 116)", totalCount)
	}
	
	return nil
}

// migration007AddArticle331 adiciona o artigo 331 (Desacato) do CP
func migration007AddArticle331(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔄 Executando migration 007: Adicionar artigo 331 (Desacato) do CP...")
	
	// Usar a função seedPenal que já tem toda a lógica correta
	// incluindo o mapeamento correto do campo IdUnico
	err := seedPenal(ctx, db, log)
	if err != nil {
		return fmt.Errorf("erro ao executar seedPenal: %w", err)
	}
	
	// Verificar se o artigo 331 foi adicionado
	coll := db.Collection("penal_artigos")
	count331, err := coll.CountDocuments(ctx, bson.M{"idUnico": "CP:331"})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao verificar artigo 331: %v", err)
	} else if count331 > 0 {
		log.Info().Msg("✅ Artigo 331 (Desacato) adicionado com sucesso!")
	} else {
		log.Warn().Msg("⚠️ Artigo 331 não foi encontrado após a migration")
	}
	
	// Verificar total geral
	totalCount, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar total: %v", err)
	} else {
		log.Info().Msgf("📊 Total de artigos no banco: %d (esperado: 117)", totalCount)
		if totalCount >= 117 {
			log.Info().Msg("✅ Total de artigos atualizado corretamente!")
		}
	}
	
	return nil
}

// migration008FixArticles47And337 corrige as descrições incorretas dos artigos 47 (LCP) e 337 (CP)
func migration008FixArticles47And337(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔄 Executando migration 008: Corrigir artigos 47 (LCP) e 337 (CP)...")
	
	// Usar a função seedPenal que já tem toda a lógica correta
	// Ela fará upsert dos artigos corrigidos automaticamente
	err := seedPenal(ctx, db, log)
	if err != nil {
		return fmt.Errorf("erro ao executar seedPenal: %w", err)
	}
	
	// Verificar se os artigos foram corrigidos
	coll := db.Collection("penal_artigos")
	
	// Verificar artigo 47 do LCP
	var artigo47 domain.ArtigoPenal
	err = coll.FindOne(ctx, bson.M{"idUnico": "LCP:47"}).Decode(&artigo47)
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao verificar artigo LCP:47: %v", err)
	} else {
		if artigo47.Descricao == "Exercício ilegal de profissão ou atividade" {
			log.Info().Msg("✅ Artigo LCP:47 corrigido com sucesso!")
		} else {
			log.Warn().Msgf("⚠️ Artigo LCP:47 ainda não está correto. Descrição atual: %s", artigo47.Descricao)
		}
	}
	
	// Verificar artigo 337 do CP
	var artigo337 domain.ArtigoPenal
	err = coll.FindOne(ctx, bson.M{"idUnico": "CP:337"}).Decode(&artigo337)
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao verificar artigo CP:337: %v", err)
	} else {
		if artigo337.Descricao == "Subtrair, reter ou inutilizar livro oficial, processo ou documento" {
			log.Info().Msg("✅ Artigo CP:337 corrigido com sucesso!")
		} else {
			log.Warn().Msgf("⚠️ Artigo CP:337 ainda não está correto. Descrição atual: %s", artigo337.Descricao)
		}
	}
	
	log.Info().Msg("✅ Migration 008 concluída!")
	return nil
}

// migration009AddArticles12_211_307_329_349 adiciona os novos artigos penais
func migration009AddArticles12_211_307_329_349(ctx context.Context, db *mongo.Database, log zerolog.Logger) error {
	log.Info().Msg("🔄 Executando migration 009: Adicionar artigos 12 (DES), 211, 307, 329 e 349 (CP)...")
	
	// Usar a função seedPenal que já tem toda a lógica correta
	err := seedPenal(ctx, db, log)
	if err != nil {
		return fmt.Errorf("erro ao executar seedPenal: %w", err)
	}
	
	// Verificar se os artigos foram adicionados
	coll := db.Collection("penal_artigos")
	
	artigosParaVerificar := []string{
		"DES:12",
		"CP:211",
		"CP:307",
		"CP:329",
		"CP:349",
	}
	
	for _, idUnico := range artigosParaVerificar {
		count, err := coll.CountDocuments(ctx, bson.M{"idUnico": idUnico})
		if err != nil {
			log.Warn().Msgf("⚠️ Erro ao verificar artigo %s: %v", idUnico, err)
		} else if count > 0 {
			log.Info().Msgf("✅ Artigo %s adicionado com sucesso!", idUnico)
		} else {
			log.Warn().Msgf("⚠️ Artigo %s não encontrado após migration.", idUnico)
		}
	}
	
	// Verificar total geral
	totalCount, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Warn().Msgf("⚠️ Erro ao contar total: %v", err)
	} else {
		log.Info().Msgf("📊 Total de artigos no banco: %d (esperado: 122)", totalCount)
		if totalCount >= 122 {
			log.Info().Msg("✅ Total de artigos atualizado corretamente!")
		}
	}
	
	log.Info().Msg("✅ Migration 009 concluída!")
	return nil
}
