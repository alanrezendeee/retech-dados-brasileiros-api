# Arquitetura: Base Própria de CEPs — Retech Core

**Data:** 02/06/2026  
**Status:** Proposta de arquitetura  
**Autor:** Retech Core · Engenharia

---

## 1. Contexto e Objetivo

Hoje a API de CEP depende 100% de fornecedores externos (ViaCEP, BrasilAPI) para dados frescos. A única persistência é um cache oportunista (Redis TTL + MongoDB `cep_cache`) — se os fornecedores ficam indisponíveis, CEPs não cacheados retornam erro.

**Objetivo:** construir base própria e relacional de todos os ~900.000 CEPs ativos do Brasil, populada de forma orgânica e por background crawling throttled, sem sobrecarregar fornecedores externos. Resultado: dependência zero de APIs externas para CEPs já conhecidos, latência <2ms, busca reversa full-text nativa.

---

## 2. Decisão Arquitetural

### Recomendação: mesmo repo, novo binário, PostgreSQL dedicado

```
retech-dados-brasileiros-api/
├── cmd/api/main.go          ← existente (API HTTP)
└── cmd/crawler/main.go      ← NOVO (processo de crawling)

PostgreSQL cepdb              ← banco novo, dedicado para CEPs
MongoDB retech_core           ← existente, mantido para tenants/auth/usage
Redis                         ← existente, mantido como L1 cache
```

**Por que não microservice separado?**

| Critério | Mesmo repo | Microservice separado |
|----------|-----------|----------------------|
| Compartilhar código de providers | ✅ direto | ❌ duplicar ou extrair lib |
| Latência API → DB | ✅ 0ms (mesmo processo) | ❌ +network hop ~2-5ms |
| CI/CD | ✅ único pipeline | ❌ novo repo, novo pipeline |
| Escala do crawler | ✅ container separado (mesmo Dockerfile multi-stage) | ✅ independente |
| Overhead operacional | ✅ mínimo | ❌ service mesh, auth inter-serviço |

**Por que PostgreSQL e não MongoDB?**

- CEP é dado **relacional puro**: UF, IBGE, DDD têm foreign key semântica implícita
- **Full-text search nativo** (`to_tsvector`) substitui busca reversa via ViaCEP
- **SKIP LOCKED** nativo → job queue sem redis/rabbitmq adicional
- **PostGIS** pronto para geo queries futuras (raio, distância, polígonos)
- **EXPLAIN ANALYZE** para otimização — MongoDB não tem equivalente
- Dados de endereço têm schema fixo, sem benefício do schema-less

---

## 3. Estrutura de Arquivos

```
retech-dados-brasileiros-api/
├── cmd/
│   ├── api/main.go                  (existente)
│   └── crawler/
│       └── main.go                  (NOVO — entry point do crawler)
│
└── internal/
    ├── cepdb/                       (NOVO — pacote completo)
    │   ├── postgres.go              (conexão pgx/v5, pool config)
    │   ├── schema.go                (DDL completo, auto-migrate na startup)
    │   ├── cep_repo.go              (Upsert, GetByCEP, Search, Stats)
    │   ├── queue_repo.go            (Enqueue, Dequeue/SKIP LOCKED, Ack, Nack)
    │   ├── crawler.go               (WorkerPool, rate limiter por provider)
    │   └── seeder.go                (seed orgânico, IBGE CSV, enumeração)
    │
    ├── http/handlers/cep.go         (MODIFICAR — L2 agora é PostgreSQL)
    └── config/config.go             (MODIFICAR — adicionar CEPDB_URL)
```

---

## 4. Schema PostgreSQL

### 4.1 Tabela principal: `ceps`

```sql
CREATE TABLE ceps (
    id              BIGSERIAL PRIMARY KEY,
    cep             CHAR(8)  NOT NULL UNIQUE,   -- normalizado sem hífen: "01310100"
    cep_formatted   CHAR(9)  NOT NULL,          -- com hífen: "01310-100"

    -- Endereço
    logradouro      TEXT,
    complemento     TEXT,
    bairro          TEXT,
    localidade      TEXT NOT NULL,
    uf              CHAR(2) NOT NULL,

    -- Códigos
    ibge            CHAR(7),    -- código IBGE município (ex: "3550308" = SP)
    gia             TEXT,       -- código GIA (específico SP)
    ddd             CHAR(2),    -- código de área telefônico
    siafi           TEXT,       -- código SIAFI (finanças públicas)

    -- Geolocalização
    latitude        DECIMAL(10,8),
    longitude       DECIMAL(11,8),

    -- Qualidade & fontes
    sources         TEXT[]   NOT NULL DEFAULT '{}',  -- provedores que confirmaram: ['viacep','brasilapi']
    quality_score   SMALLINT NOT NULL DEFAULT 0,     -- 0-100: sobe com cada confirmação de fonte
    not_found_count SMALLINT NOT NULL DEFAULT 0,     -- 404s acumulados (>3 → marcar inativo)

    -- Ciclo de vida
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '90 days'
);

-- Queries frequentes
CREATE INDEX idx_ceps_uf      ON ceps(uf);
CREATE INDEX idx_ceps_ibge    ON ceps(ibge);
CREATE INDEX idx_ceps_ddd     ON ceps(ddd);
CREATE INDEX idx_ceps_expires ON ceps(expires_at) WHERE expires_at < NOW();

-- Full-text search (busca reversa: uf + logradouro + bairro + localidade)
CREATE INDEX idx_ceps_fts ON ceps USING gin(
    to_tsvector('portuguese',
        coalesce(logradouro, '') || ' ' ||
        coalesce(bairro, '')     || ' ' ||
        localidade
    )
);
```

### 4.2 Fila de crawling: `cep_crawl_queue`

```sql
CREATE TABLE cep_crawl_queue (
    id              BIGSERIAL PRIMARY KEY,
    cep             CHAR(8)     NOT NULL UNIQUE,
    priority        SMALLINT    NOT NULL DEFAULT 0,       -- 100=user query, 50=IBGE seed, 0=enum
    attempts        SMALLINT    NOT NULL DEFAULT 0,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending|processing|done|failed
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Índice crítico para SKIP LOCKED eficiente
CREATE INDEX idx_queue_pick ON cep_crawl_queue(priority DESC, next_attempt_at ASC)
    WHERE status IN ('pending', 'failed');
```

> **Por que `cep_crawl_queue` no PostgreSQL e não Redis?**
> PostgreSQL `SELECT FOR UPDATE SKIP LOCKED` é a solução canônica para job queues persistentes em Go/Postgres. Não adiciona dependência nova, tem durabilidade (survives restarts), visibilidade via SQL e é exatamente como o Que usa o Sidekiq/PostgreSQL mode, Que usa o River, etc.

### 4.3 Log de crawling: `cep_crawl_logs`

```sql
CREATE TABLE cep_crawl_logs (
    id          BIGSERIAL   PRIMARY KEY,
    cep         CHAR(8)     NOT NULL,
    provider    VARCHAR(50) NOT NULL,
    success     BOOLEAN     NOT NULL,
    latency_ms  SMALLINT,
    error       TEXT,
    crawled_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_crawl_logs_date ON cep_crawl_logs(crawled_at DESC);
-- Retenção: deletar logs > 30 dias via job periódico
```

---

## 5. Arquitetura do Crawler

### 5.1 Fluxo geral

```
cmd/crawler/main.go
    │
    ├─ [startup] schema.AutoMigrate(pgx)
    ├─ [startup] seeder.SeedIBGE(csvPath)  ← opcional, flag --seed-ibge
    ├─ [startup] seeder.StartEnumeration() ← goroutine, gera fila em background
    │
    └─ crawler.Run(cfg)
           │
           ├── WorkerPool (N goroutines, CRAWLER_WORKERS env, default: 3)
           │       │
           │       └── loop:
           │             1. queue_repo.Dequeue()  ← SKIP LOCKED, pega 1 CEP
           │             2. limiter[provider].Wait(ctx)  ← respeita rate limit
           │             3. fetchCEP(cep, provider)      ← HTTP call
           │             4. cep_repo.Upsert(resultado)   ← salva no PostgreSQL
           │             5. redis.Set(cep, resultado, ttl) ← atualiza L1
           │             6. queue_repo.Ack(id)           ← status=done
           │             (falha: queue_repo.Nack(id, err, nextAttemptAt))
           │
           └── Requeue ticker (a cada 5min)
                   → SELECT cep FROM ceps WHERE expires_at < NOW() LIMIT 1000
                   → queue_repo.EnqueueBatch(ceps, priority=50)
```

### 5.2 Dequeue com SKIP LOCKED

```sql
-- Executado em transação por cada worker
BEGIN;
SELECT id, cep
FROM cep_crawl_queue
WHERE status IN ('pending', 'failed')
  AND next_attempt_at <= NOW()
ORDER BY priority DESC, next_attempt_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- Se retornou linha:
UPDATE cep_crawl_queue
SET status = 'processing', attempts = attempts + 1
WHERE id = $1;
COMMIT;
```

Múltiplos workers executam isso simultaneamente sem conflito. `SKIP LOCKED` pula linhas que outro worker já travou.

### 5.3 Rate limiting por provider

```go
// internal/cepdb/crawler.go
import "golang.org/x/time/rate"

var limiters = map[string]*rate.Limiter{
    "viacep":    rate.NewLimiter(rate.Every(time.Second), 1),      // 60/min
    "brasilapi": rate.NewLimiter(rate.Every(time.Second), 1),      // 60/min
    "opencep":   rate.NewLimiter(rate.Every(2*time.Second), 1),    // 30/min
}

func (c *Crawler) fetchWithLimit(ctx context.Context, provider, cep string) (*CEPData, error) {
    if err := limiters[provider].Wait(ctx); err != nil {
        return nil, err
    }
    return c.providers[provider].Fetch(ctx, cep)
}
```

### 5.4 Retry com backoff exponencial

```
attempt 1 falha → next_attempt_at = NOW() + 30s,  status='failed'
attempt 2 falha → next_attempt_at = NOW() + 5min, status='failed'
attempt 3 falha → next_attempt_at = NOW() + 1h,   status='failed'
attempt > 3    → status='failed' definitivo, last_error salvo
```

CEPs marcados como `failed` definitivo após 3 tentativas são revisitados após 30 dias pelo requeue ticker.

### 5.5 Rotação de providers

Cada worker usa um provider diferente em round-robin para distribuir carga:

```
Worker 0 → viacep primário, brasilapi fallback
Worker 1 → brasilapi primário, opencep fallback
Worker 2 → opencep primário, viacep fallback
```

Com 3 workers e 60 req/min cada: **~180 CEPs/min = 10.800/hora**.

---

## 6. Estratégias de Seed

### 6.1 Seed orgânico (começa imediatamente)

No handler `GET /cep/:codigo`, após buscar no provider externo:
```go
// internal/http/handlers/cep.go
// Após fetch bem-sucedido no provider:
go cepDB.Upsert(ctx, result, priority=100)  // salvar no PostgreSQL
go queue.Enqueue(cep, priority=100)          // re-verificar em 90 dias
```

CEPs que usuários realmente consultam têm prioridade máxima. Os top 20% dos CEPs (aeroportos, capitais, centros comerciais) cobrem ~80% das consultas — ficam populados nos primeiros dias.

### 6.2 Seed por faixas IBGE (1ª semana, priority=50)

IBGE disponibiliza faixas de CEP por município publicamente. O seeder lê um CSV e enfileira:
```
Município: São Paulo → CEPs 01000-000 a 09999-999
Município: Rio de Janeiro → CEPs 20000-000 a 28999-999
...
```

```go
// go run cmd/crawler/main.go --seed-ibge=./seeds/ibge-cep-ranges.csv
func SeedIBGE(csvPath string, queue *QueueRepo) error {
    // lê CSV, gera todos CEPs da faixa
    // queue.EnqueueBatch(ceps, priority=50)
}
```

### 6.3 Enumeração progressiva (background contínuo, priority=0)

Goroutine permanente que percorre o espaço de CEPs sistematicamente:
- Só insere na fila se `cep NOT IN (SELECT cep FROM ceps)` e `cep NOT IN (SELECT cep FROM cep_crawl_queue)`
- Velocidade: controlada pelo rate limiter — não gera fila infinita, gera sob demanda

Faixas por prioridade de enumeração (baseado em densidade populacional):
```
01000000–19999999  São Paulo
20000000–28999999  Rio de Janeiro
30000000–39999999  Minas Gerais
40000000–48999999  Bahia
...
90000000–99999999  Sul do Brasil
```

---

## 7. Integração com API Existente

### Nova lookup chain em `handlers/cep.go`

```
GET /cep/:codigo
    │
    ├── [L1] Redis        → HIT → retorna + X-Cache-Layer: redis
    │         ↓ MISS
    ├── [L2] PostgreSQL   → HIT → retorna + popula Redis + enfileira refresh se expired
    │         ↓ MISS           + X-Cache-Layer: postgres
    ├── [L3] MongoDB*     → HIT → retorna + upsert PostgreSQL + popula Redis  (*transição 90 dias)
    │         ↓ MISS           + X-Cache-Layer: mongodb-legacy
    └── [L4] Provider chain → viacep → brasilapi → opencep
                            → upsert PostgreSQL (sources, quality_score++)
                            → set Redis L1
                            → retorna + X-Cache-Layer: provider
```

MongoDB `cep_cache` vira legado durante a transição. Após 90 dias (base PostgreSQL madura), remover L3 e a collection.

### SearchCEP — upgrade completo

**Antes (atual):** chama ViaCEP externamente com uf+cidade+logradouro.
**Depois:** query PostgreSQL full-text, resposta em <5ms, zero dependência externa:

```sql
SELECT cep_formatted, logradouro, complemento, bairro, localidade, uf, ibge, ddd
FROM ceps
WHERE uf = $1
  AND to_tsvector('portuguese', coalesce(logradouro,'') || ' ' || coalesce(bairro,'') || ' ' || localidade)
      @@ plainto_tsquery('portuguese', $2)
ORDER BY quality_score DESC
LIMIT 20;
```

---

## 8. Observabilidade

### Novos endpoints admin

```
GET /admin/cepdb/stats
→ {
    total_ceps: 450000,
    coverage_pct: 50.0,
    quality_avg: 72,
    queue_pending: 12000,
    queue_processing: 3,
    queue_failed: 89,
    redis_hit_rate_pct: 45.2,
    postgres_hit_rate_pct: 38.7,
    provider_hit_rate_pct: 16.1
  }

GET /admin/cepdb/coverage
→ por UF: { "SP": { total: 120000, pct_ibge: 0.98, pct_geo: 0.12, pct_fresh: 0.87 }, ... }

GET /admin/cepdb/queue
→ { pending: 12000, processing: 3, failed: 89, estimated_completion: "2026-08-15T00:00:00Z" }

GET /admin/cepdb/crawl-logs?from=2026-06-01&provider=viacep
→ métricas de sucesso/falha por provider, latência média
```

---

## 9. Estimativa de Cobertura

| Cenário | req/hora | Cobertura 900K CEPs |
|---------|----------|---------------------|
| 1 worker, 60 req/min/provider | ~180/hora | ~208 dias |
| 3 workers, rotação 3 providers | ~540/hora | ~70 dias |
| + seed orgânico (usuários GOL) | acelerado | 80% cobertura em 30 dias* |

*Pareto: top 20% dos CEPs respondem por 80% das consultas. Esses ficam cobertos rapidamente via seed orgânico + IBGE seed.

**Projeção realista com GOL como cliente:**
- Semana 1: seed IBGE enfileira faixas das capitais → crawler popula ~100K CEPs
- Mês 1: seed orgânico (queries GOL) + crawler → 80% cobertura das capitais e hubs
- Mês 3: cobertura nacional ~70%
- Mês 6: cobertura nacional >95%

---

## 10. Dependências Novas (go.mod)

```
github.com/jackc/pgx/v5      # PostgreSQL driver (nativo, mais rápido que database/sql + lib/pq)
golang.org/x/time            # rate.Limiter para throttling de providers
```

`golang.org/x/time` provavelmente já resolvido indiretamente — verificar `go.sum`.

**Opcional (fase 2):**
```
github.com/riverqueue/river  # job queue opinado para Go+PostgreSQL (alternativa ao queue manual)
                             # usar se a fila manual ficar complexa
```

---

## 11. Variáveis de Ambiente Novas

```env
# PostgreSQL CEP DB
CEPDB_URL=postgres://user:pass@localhost:5432/cepdb?sslmode=disable
CEPDB_MAX_CONNS=20           # pool size (default: 20)

# Crawler
CRAWLER_WORKERS=3            # worker goroutines (default: 3)
CRAWLER_PROVIDERS=viacep,brasilapi,opencep   # providers em ordem de rotação
```

---

## 12. Docker Compose (adição)

```yaml
services:
  # ... serviços existentes (api, mongodb, redis) ...

  postgres-cepdb:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: cepdb
      POSTGRES_USER: retech
      POSTGRES_PASSWORD: ${CEPDB_PASSWORD}
    volumes:
      - postgres_cepdb_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  crawler:
    build:
      context: .
      dockerfile: Dockerfile.railway
      target: crawler          # multi-stage build
    environment:
      CEPDB_URL: postgres://retech:${CEPDB_PASSWORD}@postgres-cepdb:5432/cepdb
      CEP_PRIMARY_URL: ${CEP_PRIMARY_URL}
      CEP_FALLBACK_URL: ${CEP_FALLBACK_URL}
      CRAWLER_WORKERS: 3
    depends_on:
      - postgres-cepdb
    restart: unless-stopped

volumes:
  postgres_cepdb_data:
```

---

## 13. Roadmap de Implementação

### Fase 1 — Infraestrutura base (Sprint 1, ~1 semana)

1. Adicionar `pgx/v5` ao `go.mod`
2. Criar `internal/cepdb/postgres.go` — conexão + pool
3. Criar `internal/cepdb/schema.go` — DDL + auto-migrate
4. Criar `internal/cepdb/cep_repo.go` — Upsert, GetByCEP
5. Criar `internal/cepdb/queue_repo.go` — Enqueue, Dequeue (SKIP LOCKED), Ack/Nack
6. Atualizar `internal/config/config.go` — CEPDB_URL
7. Adicionar PostgreSQL ao docker-compose

### Fase 2 — Crawler (Sprint 1-2, ~1 semana)

8. Criar `internal/cepdb/crawler.go` — WorkerPool + rate limiter
9. Criar `cmd/crawler/main.go` — entry point
10. Criar `internal/cepdb/seeder.go` — seed orgânico + IBGE + enumeração
11. Testar crawler local, verificar taxa, verificar rate limiting

### Fase 3 — Integração API (Sprint 2, ~3-4 dias)

12. Modificar `internal/http/handlers/cep.go` — inserir L2 PostgreSQL na lookup chain
13. Adicionar headers `X-Cache-Layer`, `X-Cache-Age`
14. Upgrade `SearchCEP` — full-text PostgreSQL
15. Adicionar endpoints `/admin/cepdb/*`

### Fase 4 — Produção (Sprint 2-3)

16. Dockerfile multi-stage para `cmd/crawler`
17. Deploy Railway (crawler como worker separado)
18. Monitorar coverage crescendo via `/admin/cepdb/stats`
19. Após 90 dias e >95% coverage: remover MongoDB `cep_cache` (L3 legado)

---

*Documento gerado em 02/06/2026 · Retech Core Engineering*
