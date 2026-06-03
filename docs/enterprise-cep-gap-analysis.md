# Gap Analysis — API de CEP Enterprise (GOL Linhas Aéreas)

**Ref:** RTC-2026-GOL-CEP  
**Data:** 02/06/2026  
**Cliente:** GOL Linhas Aéreas S.A. — CNPJ 07.575.651/0001-59  
**Autor:** Retech Core · Engenharia

---

## 1. Resumo Executivo

A proposta RTC-2026-GOL-CEP promete uma API de CEP Enterprise com quatro pilares: cache em camadas que garante disponibilidade na Black Friday, fallback automático entre múltiplas fontes, proteção anti-robô e SLA contratual. A implementação atual atende parcialmente os pilares de cache e fallback, mas carece de circuit breaker real, sem tier enterprise no modelo de API key, sem mecanismo anti-bot além de rate limit numérico e sem infraestrutura de SLA tracking.

**Score de aderência atual estimado: 45%.**

Gaps críticos:
1. Rate limit usa MongoDB por request — gargalo sob pico Black Friday.
2. Nenhum circuit breaker — provider com falha serial continua sendo chamado.
3. Nenhum suporte a bulk/batch — GOL precisa validar múltiplos CEPs por transação.

---

## 2. Promessas da Proposta vs. Estado Atual

| Promise | Status | Observação |
|---------|--------|-----------|
| Cache em camadas (L1 Redis + L2 MongoDB) | ✅ Implementado | TTL configurável por settings admin |
| Fallback automático entre fontes | ⚠️ Parcial | Fallback existe, mas sem circuit breaker — provider com falha serial ainda é tentado primeiro |
| Proteção anti-robô | ❌ Falta | Apenas rate limit numérico; sem fingerprinting, IP allowlist ou behavioral analysis |
| SLA contratual | ❌ Falta | Sem tracking de uptime por tenant, sem alertas, sem dashboard de SLA |
| Dimensionada para pico GOL | ⚠️ Parcial | Redis pool (50 conn) e rate limit via MongoDB são gargalos sob alta carga |
| Campos enriquecidos (ibge, ddd, lat/lon) | ⚠️ Parcial | Struct tem campos, mas `ibge` e `ddd` não são populados via BrasilAPI |
| Bulk lookup para checkout | ❌ Falta | Apenas endpoint unitário — sem `POST /cep/batch` |
| Tier enterprise / quota dedicada | ❌ Falta | APIKey model sem campo de tier ou quota diferenciada |
| IP allowlist por cliente enterprise | ❌ Falta | Nenhuma restrição por IP no modelo de APIKey |
| Observabilidade: provider + cache hit | ⚠️ Parcial | `source` está na response mas não salvo no usage log; sem p95 tracking |

---

## 3. Melhorias Necessárias

### A. Cache & Performance

#### A1. TTL Index MongoDB (crítico)
**Problema:** expiração do cache MongoDB é validada manualmente no código (`time.Since(cachedAt) < ttl`). Se o processo não fizer a validação, dados expirados ficam na coleção indefinidamente.

**Solução:** criar TTL index no MongoDB na collection `cep_cache`:
```go
// internal/bootstrap/indexes.go
mongo.IndexModel{
    Keys:    bson.D{{Key: "cachedAt", Value: 1}},
    Options: options.Index().SetExpireAfterSeconds(int32(ttlSeconds)),
}
```
> **Atenção:** TTL index MongoDB não é dinamicamente configurável. Usar valor alto (ex: 90 dias) no index e manter validação manual para TTL menor. Ou usar `expireAt` field com `expireAfterSeconds: 0`.

#### A2. Rate Limit via Redis (crítico para Black Friday)
**Problema:** `middleware/rate_limiter.go` faz read+write no MongoDB por request para contar quota. Sob pico (Black Friday), isso cria contenção no MongoDB.

**Solução:** substituir MongoDB por Redis INCR + EXPIRE para contadores de rate limit:
```
INCR  rl:{tenantId}:day:{YYYY-MM-DD}     → incrementa contador diário
EXPIRE rl:{tenantId}:day:{YYYY-MM-DD} 86400
INCR  rl:{tenantId}:min:{YYYY-MM-DD-HH-MM}
EXPIRE rl:{tenantId}:min:{YYYY-MM-DD-HH-MM} 60
```
Reduz latência do middleware de ~15ms (MongoDB) para <1ms (Redis). Arquivos a modificar: `internal/middleware/rate_limiter.go`.

#### A3. Aumentar Redis Connection Pool
**Problema:** pool atual de 50 conexões (`PoolSize: 50`) pode saturar com enterprise + pico simultâneo.

**Solução:** tornar pool configurável via env:
```
REDIS_POOL_SIZE=200        # default: 50 → enterprise: 200
REDIS_MAX_RETRIES=3
REDIS_MIN_IDLE_CONNS=20
```
Arquivo: `internal/cache/redis_client.go`.

#### A4. Cache Warming
**Problema:** CEPs frequentes da GOL (aeroportos, hubs logísticos) causam cache miss no primeiro request após reinício ou TTL expiry.

**Solução:** novo endpoint admin para pré-aquecer cache:
```
POST /admin/cache/cep/warm
Body: {"ceps": ["01310-100", "04547-130", ...]}
```
Processa em background com goroutines (semáforo de 10 concurrent). Arquivo: `internal/http/handlers/cep.go`.

#### A5. Headers de Cache na Response
**Problema:** cliente não sabe se recebeu dado do cache ou do provider externo.

**Solução:** adicionar headers:
```
X-Cache-Hit: true
X-Cache-Layer: redis | mongodb | provider
X-Cache-Age: 3600          (segundos desde cachedAt)
X-Provider: viacep | brasilapi | opencep
```

---

### B. Provider Chain & Resiliência

#### B1. Circuit Breaker por Provider (crítico)
**Problema:** código atual faz `fetchViaCEP` → falha → `fetchBrasilAPI`. Mas no próximo request, tenta ViaCEP novamente mesmo que ele esteja com falha consecutiva, desperdiçando latência.

**Solução:** implementar circuit breaker simples por provider usando Redis:
```
Estado: CLOSED (normal) | OPEN (falha, skip) | HALF-OPEN (teste)

Key Redis: cb:provider:{viacep|brasilapi}
Valor: {"state":"OPEN","failCount":5,"openedAt":"..."}
TTL: 30 segundos (janela de recuperação)

Lógica:
- 5 falhas consecutivas → estado OPEN por 30s
- Em OPEN: pula direto para próximo provider
- Após 30s → HALF-OPEN: tenta 1 request de teste
- Sucesso no teste → volta CLOSED; falha → OPEN por mais 30s
```
Novo arquivo: `internal/cache/circuit_breaker.go`. Usar no `handlers/cep.go`.

#### B2. Terceiro Provider (OpenCEP)
**Problema:** com apenas 2 providers, se ambos falharem simultaneamente (improvável mas possível), retorna 404.

**Solução:** adicionar OpenCEP como terceiro fallback:
```
CEP_TERTIARY_PROVIDER=opencep
CEP_TERTIARY_URL=https://opencep.com
```
Mapeamento: `GET https://opencep.com/v1/{cep}` → `{cep, logradouro, complemento, bairro, localidade, uf, ibge}`.
Arquivos: `internal/config/apis.go`, `internal/http/handlers/cep.go`.

#### B3. Completar Mapeamento BrasilAPI
**Problema:** struct `CEPResponse` tem campos `IBGE` e `DDD`, mas mapeamento do BrasilAPI não os popula.

**Solução:** BrasilAPI retorna:
```json
{"cep":"01310-100","state":"SP","city":"São Paulo","neighborhood":"Bela Vista",
 "street":"Avenida Paulista","service":"open-cep","ibge":"3550308"}
```
Adicionar `ibge` ao mapeamento em `fetchBrasilAPI`. Arquivo: `internal/http/handlers/cep.go`.

#### B4. Timeout Independente por Provider
**Problema:** `config.GetCEPTimeout()` aplica mesmo timeout para todos os providers. Provider lento segura o circuito inteiro.

**Solução:**
```
CEP_PRIMARY_TIMEOUT=3s     (ViaCEP geralmente <500ms)
CEP_FALLBACK_TIMEOUT=5s    (BrasilAPI pode ser mais lento)
CEP_TERTIARY_TIMEOUT=4s
```

---

### C. Bulk/Batch Lookup

#### C1. POST /cep/batch
**Problema:** GOL valida endereços em checkout de passagem — múltiplos CEPs por transação. Hoje precisa de N requests sequenciais.

**Solução:** novo endpoint batch:
```
POST /cep/batch
X-API-Key: ...

Body:
{
  "ceps": ["01310-100", "20040-020", "30140-110"]
}

Response 200:
{
  "results": [
    {"cep": "01310-100", "logradouro": "...", ...},
    {"cep": "20040-020", "logradouro": "...", ...}
  ],
  "errors": [
    {"cep": "00000-000", "error": "CEP não encontrado"}
  ],
  "meta": {
    "total": 3,
    "success": 2,
    "errors": 1,
    "cache_hits": 2
  }
}
```

Limites: max 50 CEPs por request. Processamento paralelo com semáforo (max 10 goroutines para providers externos). Arquivo: `internal/http/handlers/cep.go`. Rota nova em `internal/http/router.go`.

---

### D. Enterprise API Key

#### D1. Campo Tier no APIKey
**Problema:** `domain.APIKey` não diferencia plano free de enterprise. GOL enterprise deve ter quota maior, IP allowlist e prioridade.

**Solução:** adicionar campos ao `domain.APIKey`:
```go
// internal/domain/apikey.go
type APIKey struct {
    // ... campos existentes ...
    Tier        string   `bson:"tier" json:"tier"`               // free | pro | enterprise
    AllowedIPs  []string `bson:"allowedIPs" json:"allowedIPs"`   // se vazio, sem restrição
    DailyQuota  int64    `bson:"dailyQuota" json:"dailyQuota"`    // 0 = usa padrão do tier
    MinuteQuota int64    `bson:"minuteQuota" json:"minuteQuota"`  // 0 = usa padrão do tier
}
```

Quotas por tier (configurar em settings):
```
free:       1.000 req/dia,  60 req/min
pro:       50.000 req/dia, 500 req/min
enterprise: ilimitado ou custom (campo DailyQuota)
```

#### D2. Middleware IP Allowlist
**Problema:** nenhuma validação de IP por client enterprise.

**Solução:** middleware que roda após `AuthAPIKey`, antes de rate limit:
```go
// internal/auth/ip_allowlist.go
func IPAllowlist(apikeys *storage.APIKeysRepo) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.GetString("api_key_obj") // passar objeto, não só string
        if len(key.AllowedIPs) == 0 {
            c.Next()
            return
        }
        clientIP := extractIP(c)
        for _, allowed := range key.AllowedIPs {
            if clientIP == allowed { c.Next(); return }
        }
        c.JSON(403, gin.H{"error": "IP não autorizado"})
        c.Abort()
    }
}
```

---

### E. Anti-bot & Segurança

#### E1. Salvar Fingerprint no Usage Log
**Problema:** header `X-Browser-Fingerprint` já é enviado pelos clientes mas não é salvo no `api_usage_logs`.

**Solução:** adicionar campos ao `domain.APIUsageLog`:
```go
Fingerprint string `bson:"fingerprint,omitempty" json:"fingerprint,omitempty"`
Referer     string `bson:"referer,omitempty" json:"referer,omitempty"`
```
Capturar em `middleware/usage_logger.go`. Habilita análise forense pós-incidente.

#### E2. Behavioral Rate Limiting
**Problema:** bot pode consultar CEPs sequencialmente (00000-000, 00000-001, ...) dentro do rate limit numérico.

**Solução:** detector de padrão sequencial em Redis:
```
Key: pattern:{tenantId}:seq   → lista dos últimos 20 CEPs consultados (LPUSH + LTRIM)
TTL: 60 segundos

Lógica: se >= 15 dos últimos 20 CEPs forem numérica e sequencialmente crescentes → 
        responder 429 com mensagem específica de padrão suspeito
```
Implementar como check opcional no handler, após cache hit (não aplica custo extra para requests legítimos).

#### E3. Request Signing para Enterprise (opcional)
**Problema:** API Key pode ser interceptada. Enterprise pode querer assinatura de request.

**Solução:** header opcional `X-Signature`:
```
X-Signature: HMAC-SHA256(timestamp + "." + method + "." + path, signingSecret)
X-Timestamp: 1748880000
```
Middleware valida se timestamp é recente (<5min) e assinatura correta. Opt-in via campo `RequireSigning bool` no APIKey.

---

### F. Observabilidade & SLA

#### F1. Campos Extras no Usage Log
**Problema:** `api_usage_logs` não registra cache hit, layer ou qual provider respondeu — impossível calcular cache hit rate ou custo de providers.

**Solução:** adicionar campos ao `domain.APIUsageLog`:
```go
CacheHit          bool   `bson:"cacheHit" json:"cacheHit"`
CacheLayer        string `bson:"cacheLayer,omitempty" json:"cacheLayer,omitempty"` // redis|mongodb
Provider          string `bson:"provider,omitempty" json:"provider,omitempty"`     // viacep|brasilapi
ProviderLatencyMs int64  `bson:"providerLatencyMs,omitempty" json:"providerLatencyMs,omitempty"`
BatchSize         int    `bson:"batchSize,omitempty" json:"batchSize,omitempty"`   // para /batch
```
Passar via context do handler para o middleware de logging.

#### F2. Analytics CEP Admin
**Problema:** nenhum endpoint agrega métricas de CEP com cache hit rate ou latência percentual.

**Solução:** novo endpoint:
```
GET /admin/analytics/cep?from=2026-06-01&to=2026-06-30&tenantId=xxx

Response:
{
  "period": "2026-06-01/2026-06-30",
  "totalRequests": 1500000,
  "cacheHitRate": 0.87,
  "cacheHitByLayer": {"redis": 0.72, "mongodb": 0.15},
  "providerBreakdown": {"viacep": 0.11, "brasilapi": 0.02},
  "latencyP50Ms": 3,
  "latencyP95Ms": 45,
  "latencyP99Ms": 120,
  "errorsRate": 0.001
}
```
Agregação via MongoDB `$group` + `$percentile` (MongoDB 7+).

#### F3. SLA Tracking por Tenant
**Problema:** sem capacidade de gerar relatório de SLA para cliente enterprise.

**Solução:** nova coleção `sla_snapshots` — job periódico (a cada hora) salva:
```go
type SLASnapshot struct {
    TenantID     string    `bson:"tenantId"`
    Hour         string    `bson:"hour"`     // 2026-06-02T14
    TotalReqs    int64     `bson:"totalReqs"`
    ErrorReqs    int64     `bson:"errorReqs"`
    AvailPct     float64   `bson:"availPct"` // (1 - errors/total) * 100
    P95Ms        float64   `bson:"p95Ms"`
}

GET /admin/sla/:tenantId?from=...&to=...
Response: uptime%, SLA breaches, p95 trend
```

---

## 4. Matriz de Prioridade

### Sprint 1 — Crítico para assinar contrato (2-3 semanas)

| # | Melhoria | Impacto | Esforço | Arquivo principal |
|---|---------|---------|---------|------------------|
| 1 | Rate limit via Redis (A2) | 🔴 Alto | Médio | `middleware/rate_limiter.go` |
| 2 | Circuit breaker por provider (B1) | 🔴 Alto | Médio | novo: `cache/circuit_breaker.go` |
| 3 | POST /cep/batch (C1) | 🔴 Alto | Médio | `handlers/cep.go`, `http/router.go` |
| 4 | Campo Tier + AllowedIPs no APIKey (D1) | 🔴 Alto | Baixo | `domain/apikey.go` |
| 5 | Completar mapeamento ibge BrasilAPI (B3) | 🟡 Médio | Baixo | `handlers/cep.go` |
| 6 | Headers X-Cache-* na response (A5) | 🟡 Médio | Baixo | `handlers/cep.go` |
| 7 | Campos extras no usage log (F1) | 🟡 Médio | Baixo | `domain/api_usage_log.go`, `middleware/usage_logger.go` |

### Sprint 2 — Completar tier Enterprise (2 semanas)

| # | Melhoria | Impacto | Esforço | Arquivo principal |
|---|---------|---------|---------|------------------|
| 8 | Middleware IP allowlist (D2) | 🟡 Médio | Baixo | novo: `auth/ip_allowlist.go` |
| 9 | TTL index MongoDB cep_cache (A1) | 🟡 Médio | Baixo | `bootstrap/indexes.go` |
| 10 | Terceiro provider OpenCEP (B2) | 🟡 Médio | Médio | `config/apis.go`, `handlers/cep.go` |
| 11 | Timeout independente por provider (B4) | 🟡 Médio | Baixo | `config/apis.go` |
| 12 | Cache warming endpoint (A4) | 🟡 Médio | Médio | `handlers/cep.go`, `http/router.go` |
| 13 | Redis pool configurável (A3) | 🟡 Médio | Baixo | `cache/redis_client.go` |
| 14 | Analytics CEP admin (F2) | 🟡 Médio | Alto | `handlers/admin.go` |

### Sprint 3 — Diferencial Enterprise (2 semanas)

| # | Melhoria | Impacto | Esforço | Arquivo principal |
|---|---------|---------|---------|------------------|
| 15 | SLA tracking + endpoint (F3) | 🟡 Médio | Alto | novo: `domain/sla_snapshot.go`, `storage/sla_repo.go` |
| 16 | Fingerprint no usage log (E1) | 🟢 Baixo | Baixo | `middleware/usage_logger.go` |
| 17 | Behavioral rate limiting (E2) | 🟢 Baixo | Médio | `handlers/cep.go` |
| 18 | Request signing (E3) | 🟢 Baixo | Médio | novo: `auth/signing_middleware.go` |

---

## 5. Estimativa de Esforço Total

| Sprint | Escopo | Estimativa |
|--------|--------|-----------|
| Sprint 1 | Gargalos performance + batch + tier model | 2–3 semanas |
| Sprint 2 | Enterprise features completas | +2 semanas |
| Sprint 3 | Segurança avançada + SLA dashboard | +2 semanas |
| **Total** | **Feature-complete Enterprise** | **6–7 semanas** |

---

## 6. Riscos

| Risco | Probabilidade | Mitigação |
|-------|-------------|-----------|
| Redis indisponível → rate limit cai | Médio | Manter fallback MongoDB como safety net (degraded mode) |
| ViaCEP + BrasilAPI simultâneos offline | Baixo | Terceiro provider (B2) + MongoDB cache L2 cobre maioria dos CEPs já consultados |
| MongoDB `$percentile` requer versão 7+ | Médio | Verificar versão em produção; fallback para cálculo manual via sort + skip |
| Black Friday: Redis pool esgota | Baixo | Pool 200 + horizontal scaling Redis (sentinel/cluster) |
| GOL exige uptime 99.99% | Alto | Múltiplos providers + circuit breaker + cache L2 devem garantir 99.9%; para 99.99% precisar de infraestrutura redundante |

---

*Documento gerado em 02/06/2026 · Retech Core Engineering*
