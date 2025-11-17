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

