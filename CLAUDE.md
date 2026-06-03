# Retech Core API — Sub-Agente Backend

## Identidade

Go 1.24, Gin framework — API gateway de dados brasileiros (GEO, CEP, CNPJ, Penal) com multi-tenancy, autenticação dupla (API Key + JWT), rate limiting, cache Redis e observabilidade estruturada.

Módulo: `github.com/theretech/retech-core`

---

## Role: Sub-Agente Backend

Este agente recebe specs do agente principal (`retech-dados-brasileiros-admin`) e implementa no backend. Sempre ler a spec recebida antes de agir.

---

## Arquitetura

```
cmd/api/main.go                   # Entry point

internal/
├── http/
│   ├── router.go                 # Todas as rotas e middleware stack
│   └── handlers/                 # Um handler por domínio
│       ├── geo.go                # GET /geo/ufs, /geo/municipios
│       ├── cep.go                # GET /cep/:codigo
│       ├── cnpj.go               # GET /cnpj/:numero
│       ├── penal.go              # GET /penal/artigos
│       ├── auth.go               # POST /auth/login, /register, /refresh
│       ├── apikey.go             # CRUD /me/apikeys, /admin/apikeys
│       ├── tenant.go             # CRUD /admin/tenants
│       ├── admin.go              # GET /admin/stats, /analytics
│       └── settings.go          # GET|PUT /admin/settings
├── domain/                       # Structs de domínio (MongoDB documents)
├── storage/                      # Repos MongoDB (um arquivo por domínio)
├── auth/                         # JWT service + middlewares
├── middleware/                   # CORS, rate limit, logging, usage logger
├── cache/                        # Redis client + settings cache
├── bootstrap/                    # Migrations, seeds, indexes
├── config/                       # ENV vars loader
├── observability/                # zerolog setup
└── utils/                        # Activity logging helpers
```

---

## Adicionar Nova Feature

1. Domain struct em `internal/domain/<nome>.go`
2. MongoDB repo em `internal/storage/<nome>_repo.go`
3. Handler em `internal/http/handlers/<nome>.go`
4. Registrar rotas em `internal/http/router.go`
5. Migration/seed se necessário em `internal/bootstrap/migrations.go`

---

## Auth

| Middleware | Header | Onde usar |
|-----------|--------|-----------|
| `auth.AuthAPIKey(apikeys)` | `X-API-Key` | Endpoints públicos com chave |
| `auth.AuthJWT(jwtService)` | `Authorization: Bearer <token>` | Rotas autenticadas |
| `auth.RequireSuperAdmin()` | — | Rotas `/admin/*` |
| `auth.RequireTenantUser()` | — | Rotas `/me/*` |

---

## Middleware Stack (ordem em router.go)

CORS → Maintenance → Auth → Scope → RateLimit → UsageLogger → RequestID → Logging → Recover

---

## Padrões Obrigatórios

**Response envelope:**
```go
c.JSON(http.StatusOK, gin.H{"data": result})
```

**Erros (RFC 7807):**
```go
c.JSON(http.StatusBadRequest, gin.H{
    "type":   "about:blank",
    "title":  "Bad Request",
    "status": 400,
    "detail": "mensagem clara",
})
```

**Multi-tenancy:** toda entidade nova deve referenciar `tenant_id`.

**Redis cache:** graceful degradation — se Redis indisponível, continua sem cache.

---

## Configuração

Variáveis chave (ver `env.example` para lista completa):

```
PORT=8080
MONGO_URI=mongodb://localhost:27017
MONGO_DB=retech_core
REDIS_URL=redis://localhost:6379
JWT_ACCESS_SECRET=...
JWT_REFRESH_SECRET=...
APIKEY_HASH_SECRET=...
```

---

## Dev

```bash
make up      # Sobe containers (API + MongoDB)
make logs    # Logs da API
make admin   # Cria super admin (alanrezendeee@gmail.com / admin123456)
make build   # Rebuild container
make down    # Para containers
```

API sobe em `http://localhost:8080`. Docs: `http://localhost:8080/docs`.
