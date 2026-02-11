# retech-core — Core APIs for The Retech - 

Centraliza **serviços utilitários** e **integrações públicas** para acelerar diversos projetos (web, mobile, backoffice). Foco em **estabilidade**, **observabilidade** e **padronização** de respostas.

**Domínio**: `core.theretech.com.br`  
**Versão**: 0.4.0  
**Status**: 🟢 Em produção

📚 **[Índice completo de documentação](docs/INDEX.md)**

---

## ✨ Objetivos

* Expor endpoints **estáveis** para dados públicos/derivados (UFs, municípios, RAs do DF, CEP, bancos, feriados etc.).
* **Padronizar** contratos (envelope de resposta, erros RFC 7807) entre projetos.
* **Desacoplar** front-ends das fontes externas (IBGE, BrasilAPI, etc.), com **cache** e **rate limiting**.
* **Observabilidade** pronta (Prometheus + OpenTelemetry) e **resiliência** (retries, circuit breaker).

---

## 📐 Arquitetura (resumo)

```
Cliente → retech-core (API Gateway utilitário)
            ├─ /geo/*   → (cache + normalização) → IBGE/BrasilAPI/fonte DF
            ├─ /cep/*   → (cache + fallback) → ViaCEP/BrasilAPI
            ├─ /utils/* → serviços puros (CPF/CNPJ, phone, currency, slug)
            ├─ /biz/*   → bancos, feriados, etc. (cache)
            └─ /health,/metrics,/docs
```

* **Cache**: Redis (TTL configurável).
* **Circuit breaker**: impedir cascata de falhas de upstream.
* **Rate limiting**: token bucket por IP/chave.
* **Auth**: API Key opcional por rota; JWT passthrough quando aplicável.

---

## 📦 Estrutura de pastas (Go + Gin)

```
retech-core/
  cmd/api/main.go
  internal/
    http/ (rotas, handlers)
    services/ (casos de uso)
    clients/ (ibge, brasilapi, viacep, gdf)
    cache/ (redis)
    config/
    observability/ (otel, prometheus)
    middleware/ (auth, rate-limit, cors, idempotency)
    domain/ (DTOs/validators)
  pkg/
  build/
    Dockerfile
    docker-compose.yml
```

---

## 🔐 Segurança

* **API Key** por projeto/cliente (header `x-api-key`).
* **JWT passthrough** (quando consumir serviços internos que exigem identidade).
* **CORS** configurável por ambiente.
* **Auditoria** básica (request-id, caller, rota, status, latência).

---

## 📊 Observabilidade

* **/metrics** (Prometheus)
* **OpenTelemetry** traces (HTTP client/server)
* **Log estruturado** (JSON; correlação por `X-Request-ID`)

---

## 📑 Convenções de resposta

### Sucesso

```json
{
  "success": true,
  "code": "OK",
  "data": { "..." },
  "meta": { "cache": {"hit": true, "ttl": 300} }
}
```

### Erro (RFC 7807)

```json
{
  "type": "https://retech-core/errors/upstream-timeout",
  "title": "Upstream Timeout",
  "status": 504,
  "detail": "IBGE não respondeu em 5s",
  "instance": "/geo/ufs?q=pern",
  "traceId": "01H..."
}
```

---

## 🚀 Endpoints implementados (v1)

### 0) Infra

* ✅ `GET /health` → Verifica saúde da aplicação e conexão com MongoDB
* ✅ `GET /version` → Retorna versão da API
* ✅ `GET /docs` → Documentação HTML (Redoc)
* ✅ `GET /openapi.yaml` → Especificação OpenAPI

### 1) Tenants

* ✅ `POST /tenants` → Criar tenant
* ✅ `GET /tenants` → Listar tenants
* ✅ `GET /tenants/:id` → Buscar tenant por ID
* ✅ `PUT /tenants/:id` → Atualizar tenant
* ✅ `DELETE /tenants/:id` → Remover tenant

### 2) API Keys

* ✅ `POST /apikeys` → Criar API key
* ✅ `POST /apikeys/revoke` → Revogar API key
* ✅ `POST /apikeys/refresh` → Rotacionar API key

### 3) GEO (Estados e Municípios)

* ✅ `GET /geo/ufs` → Lista todos os estados
  **Query**: `q` (opcional, busca parcial por nome ou sigla)
  **Resposta**:

  ```json
  {
    "success": true,
    "code": "OK",
    "data": [
      {
        "id": 26,
        "sigla": "PE",
        "nome": "Pernambuco",
        "regiao": {
          "id": 2,
          "sigla": "NE",
          "nome": "Nordeste"
        }
      }
    ]
  }
  ```

  **Fonte**: Seed local baseado em dados do IBGE.

* ✅ `GET /geo/ufs/:sigla` → Busca estado específico pela sigla
  **Exemplo**: `/geo/ufs/PE`

* ✅ `GET /geo/municipios` → Lista todos os municípios
  **Query**: 
  - `uf` (opcional, filtra por estado)
  - `q` (opcional, busca por nome)
  
  **Exemplo**: `/geo/municipios?uf=PE&q=recife`

* ✅ `GET /geo/municipios/:uf` → Lista municípios de um estado
  **Exemplo**: `/geo/municipios/PE`
  **Resposta**: array de municípios com id (IBGE), nome, microrregião, mesorregião e região imediata/intermediária.

* ✅ `GET /geo/municipios/id/:id` → Busca município pelo código IBGE
  **Exemplo**: `/geo/municipios/id/2611606` (Recife)

---

## 📋 Endpoints planejados (futuro)

### CEP

* `GET /cep/{cep}` → Busca informações de CEP
  **Fonte** primária: BrasilAPI. **Fallback**: ViaCEP. **Cache** agressivo (7–30d).

### Documentos & utilidades

* `POST /utils/cpf/validate` → Validar CPF (offline)
* `POST /utils/cnpj/validate` → Validar CNPJ (offline)
* `POST /utils/phone/format` → Formatar telefone brasileiro
* `POST /utils/slugify` → Gerar slug a partir de texto

### Negócio

* `GET /biz/bancos` → Lista bancos ativos (código, nome)
* `GET /biz/feriados/{ano}` → Feriados nacionais e por UF

### GEO Avançado

* `GET /geo/df/ras` → Regiões Administrativas do DF (33 RAs)

---

## 📮 Testando com Postman

Uma **collection completa do Postman** está disponível para facilitar os testes:

- 📁 `postman_collection.json` - Collection com todos os endpoints
- 🌍 `postman_environment.json` - Environment pré-configurado para localhost
- 📖 [POSTMAN.md](POSTMAN.md) - Guia completo de uso

### Features da Collection:

- ✅ **50+ requisições** organizadas por categoria
- ✅ **Auto-save de API Keys** - Scripts automáticos salvam keys nas variáveis
- ✅ **Exemplos de casos de uso** reais
- ✅ **Documentação inline** em cada requisição
- ✅ **Environment pré-configurado** para desenvolvimento local

[👉 Ver guia completo do Postman](POSTMAN.md)

---

## 🧰 Exemplo de uso (cURL)

```bash
# Health check
curl -s 'http://localhost:8080/health' | jq

# Versão da API
curl -s 'http://localhost:8080/version' | jq

# Listar todos os estados
curl -s 'http://localhost:8080/geo/ufs' | jq

# Buscar estado específico
curl -s 'http://localhost:8080/geo/ufs/PE' | jq

# Buscar estados (filtro)
curl -s 'http://localhost:8080/geo/ufs?q=pernambuco' | jq

# Listar municípios de um estado
curl -s 'http://localhost:8080/geo/municipios/PE' | jq

# Buscar municípios por nome
curl -s 'http://localhost:8080/geo/municipios?uf=PE&q=recife' | jq

# Buscar município por código IBGE
curl -s 'http://localhost:8080/geo/municipios/id/2611606' | jq

# Criar tenant
curl -X POST 'http://localhost:8080/tenants' \
  -H 'Content-Type: application/json' \
  -d '{"tenantId":"cliente-1","name":"Cliente Exemplo","email":"contato@exemplo.com","active":true}' | jq

# Criar API key
curl -X POST 'http://localhost:8080/apikeys' \
  -H 'Content-Type: application/json' \
  -d '{"tenantId":"cliente-1","name":"Chave Producao"}' | jq
```

---

## 🧪 Contratos (OpenAPI)

A documentação completa está disponível em:
- `/docs` - Interface Redoc
- `/openapi.yaml` - Especificação OpenAPI

### Exemplo de schema de resposta

```yaml
# Resposta de sucesso
SuccessResponse:
  type: object
  properties:
    success:
      type: boolean
      example: true
    code:
      type: string
      example: "OK"
    data:
      type: object
    meta:
      type: object

# Resposta de erro (RFC 7807)
ErrorResponse:
  type: object
  properties:
    type:
      type: string
      example: "https://retech-core/errors/not-found"
    title:
      type: string
      example: "Not Found"
    status:
      type: integer
      example: 404
    detail:
      type: string
      example: "Estado não encontrado"
    instance:
      type: string
      example: "/geo/ufs/XX"
    traceId:
      type: string
```

---

## ⚙️ Configuração (ENV)

```bash
# Servidor
PORT=8080                              # Porta HTTP (padrão: 8080)
ENV=development                        # Ambiente: development | production

# MongoDB
MONGO_URI=mongodb://localhost:27017    # URI de conexão MongoDB
MONGO_DB=retech_core                   # Nome do banco de dados

# CORS
CORS_ENABLE=true                       # Habilita CORS (padrão: true)
```

### Exemplo de .env

```bash
PORT=8080
ENV=development
MONGO_URI=mongodb://mongo:27017
MONGO_DB=retech_core
CORS_ENABLE=true
```

---

## 💾 Migrations e Seeds

O sistema possui um gerenciador de migrations que executa automaticamente na inicialização da aplicação.

### Como funciona

1. Na inicialização, o sistema verifica quais migrations ainda não foram executadas
2. Executa as migrations pendentes em ordem
3. Registra cada migration executada na collection `migrations`
4. Seeds de estados e municípios são carregados automaticamente se não existirem

### Estrutura de dados

#### Estados (27 UFs)
- Seed: `seeds/estados.json`
- Collection: `estados`
- Total: 27 registros
- Fonte: IBGE

#### Municípios (5570 municípios)
- Seed: `seeds/municipios.json`
- Collection: `municipios`
- Total: 5570 registros
- Fonte: IBGE

### Localização dos arquivos de seed

O sistema busca os arquivos JSON nas seguintes localizações:
1. `seeds/` (recomendado)
2. `~/Downloads/` (conveniente para desenvolvimento)
3. `data/`
4. Raiz do projeto

### Re-executar seeds

Para re-executar os seeds:

```bash
# Conectar ao MongoDB
mongo retech_core

# Remover registros de migration
db.migrations.deleteOne({version: "001_seed_estados"})
db.migrations.deleteOne({version: "002_seed_municipios"})

# Limpar dados (opcional)
db.estados.deleteMany({})
db.municipios.deleteMany({})

# Reiniciar a aplicação
```

---

## 🚀 Deploy em Produção

### Railway (Recomendado) 🚂

Deploy simplificado com MongoDB gerenciado e CI/CD automático:

- ✅ **Deploy automático** via Git push
- ✅ **MongoDB incluído** (ou use MongoDB Atlas)
- ✅ **Seeds automáticos** na primeira execução
- ✅ **HTTPS grátis** e domínio customizável
- ✅ **Logs em tempo real**
- ✅ **Free tier disponível**

[👉 Ver guia completo de deploy no Railway](RAILWAY_DEPLOY.md)

**Quick Start:**
```bash
# 1. Verificar se está pronto
./railway-check.sh

# 2. Commit seeds
git add seeds/*.json
git commit -m "chore: adicionar seeds para produção"
git push origin main

# 3. Deploy no Railway (via dashboard ou CLI)
railway up
```

**Arquivos de configuração:**
- `Dockerfile.railway` - Dockerfile otimizado para produção
- `railway.json` / `railway.toml` - Configuração Railway
- `railway.env.example` - Variáveis de ambiente

---

## 🐳 Deploy (Docker Local)

**Dockerfile (Go)**

```dockerfile
FROM golang:1.22 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/retech-core ./cmd/api

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/retech-core /retech-core
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/retech-core"]
```

**docker-compose.yml (dev)**

```yaml
version: '3.9'
services:
  api:
    build: .
    ports: ["8080:8080"]
    env_file: .env
    depends_on: [redis]
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
```

---

## 🛡️ Boas práticas

* **Timeouts** e **retries** exponenciais para upstreams.
* **Cache key** determinística e invalidação por rota.
* **Schema validation** (ex.: go-playground/validator) em inputs.
* **E2E contract tests** (Dredd/Pact) para não quebrar clientes.
* **Idempotência** em POSTs sensíveis (chave idempotency em header).

---

## 🗺️ Roadmap

### ✅ Implementado

* [x] Sistema de migrations/seeds automático
* [x] Endpoints de estados (UFs) com busca
* [x] Endpoints de municípios com filtros por UF e nome
* [x] Gestão de tenants e API keys
* [x] Health check e versão
* [x] Documentação OpenAPI/Redoc
* [x] Índices MongoDB para performance

### 🚧 Em planejamento

#### Curto prazo
* [ ] Cache com Redis (estados e municípios)
* [ ] Rate limiting por API key
* [ ] Endpoints de CEP (integração BrasilAPI + ViaCEP)
* [ ] Middleware de autenticação obrigatória em rotas protegidas

#### Médio prazo
* [ ] Validadores de CPF/CNPJ (offline)
* [ ] Formatador de telefone brasileiro
* [ ] Utilidades (slugify, normalização)
* [ ] Endpoints de bancos (Bacen)
* [ ] Feriados nacionais e por UF

#### Longo prazo
* [ ] Regiões Administrativas do DF (33 RAs)
* [ ] CNAEs e naturezas jurídicas
* [ ] Geocoding (Nominatim, rate-limit estrito)
* [ ] Endpoints bulk para grandes volumes
* [ ] Webhooks para invalidação de cache
* [ ] Prometheus metrics
* [ ] OpenTelemetry traces

---

## 📄 Licença

Definir conforme política interna (MIT/Proprietária).

---

---

## 📚 Estrutura do projeto

```
retech-core/
├── cmd/api/
│   └── main.go                    # Ponto de entrada da aplicação
├── internal/
│   ├── auth/                      # Middleware de autenticação
│   ├── bootstrap/
│   │   ├── indexes.go             # Criação de índices MongoDB
│   │   └── migrations.go          # Sistema de migrations/seeds
│   ├── config/
│   │   └── config.go              # Configurações (env vars)
│   ├── domain/
│   │   ├── apikey.go              # Modelo de API Keys
│   │   ├── estado.go              # Modelo de Estados
│   │   ├── municipio.go           # Modelo de Municípios
│   │   └── tenant.go              # Modelo de Tenants
│   ├── http/
│   │   ├── handlers/
│   │   │   ├── apikey.go          # Handlers de API keys
│   │   │   ├── geo.go             # Handlers de GEO (estados/municípios)
│   │   │   ├── health.go          # Health check
│   │   │   ├── tenant.go          # Handlers de tenants
│   │   │   └── version.go         # Versão da API
│   │   └── router.go              # Configuração de rotas
│   ├── middleware/                # Middlewares HTTP
│   ├── observability/
│   │   └── logger.go              # Logger estruturado (zerolog)
│   └── storage/
│       ├── apikeys_repo.go        # Repositório de API Keys
│       ├── estados_repo.go        # Repositório de Estados
│       ├── mongo.go               # Cliente MongoDB
│       ├── municipios_repo.go     # Repositório de Municípios
│       └── tenants_repo.go        # Repositório de Tenants
├── seeds/
│   ├── estados.json               # Seed de estados (27 UFs)
│   └── municipios.json            # Seed de municípios (5570)
├── build/
│   ├── Dockerfile                 # Dockerfile de produção
│   └── docker-compose.yml         # Compose para desenvolvimento
└── go.mod                         # Dependências Go
```

---

## 📝 Notas de implementação

### Estados e Municípios
* Dados carregados via **seed automático** na inicialização
* Seeds baseados em dados oficiais do IBGE
* Sistema de migrations garante que seeds rodem apenas uma vez
* Índices MongoDB otimizam buscas por ID, sigla e nome
* Busca por nome implementada como **filtro local** (case-insensitive)

### Tenants e API Keys
* Sistema multi-tenant implementado
* API Keys com suporte a rotação e revogação
* Cada API key vinculada a um tenant específico

### Observabilidade
* Logs estruturados com zerolog
* Health check integrado com MongoDB
* Versão da API exposta via endpoint

### Performance
* Índices únicos: estados (id, sigla), municípios (id)
* Índices de busca: municípios (nome, UF)
* Seeds carregados em lotes de 1000 para otimizar memória
