# Seeds - Dados para Popular o Banco

Este diretório deve conter os arquivos JSON com dados para popular o banco de dados.

## Arquivos Necessários

- `estados.json` - Lista de estados brasileiros (27 estados)
- `municipios.json` - Lista de municípios brasileiros (5570 municípios)
- `penal.json` - Dispositivos penais (Código Penal completo + legislação especial) — ver seção abaixo

## Como Usar

1. Coloque os arquivos `estados.json` e `municipios.json` neste diretório
2. Execute a aplicação normalmente
3. O sistema detectará automaticamente os arquivos e executará as migrations/seeds

## Localização dos Arquivos

O sistema busca os arquivos nas seguintes localizações (em ordem):

1. Diretório `seeds/` (este diretório)
2. `~/Downloads/` (conveniente para desenvolvimento)
3. Diretório `data/`
4. Diretório raiz do projeto

## Formato dos Arquivos

### estados.json

```json
[
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
```

### municipios.json

```json
[
  {
    "id": 2611606,
    "nome": "Recife",
    "microrregiao": {
      "id": 26017,
      "nome": "Recife",
      "mesorregiao": {
        "id": 2605,
        "nome": "Metropolitana de Recife",
        "UF": {
          "id": 26,
          "sigla": "PE",
          "nome": "Pernambuco",
          "regiao": {
            "id": 2,
            "sigla": "NE",
            "nome": "Nordeste"
          }
        }
      }
    },
    "regiao-imediata": {
      "id": 260001,
      "nome": "Recife",
      "regiao-intermediaria": {
        "id": 2601,
        "nome": "Recife",
        "UF": {
          "id": 26,
          "sigla": "PE",
          "nome": "Pernambuco",
          "regiao": {
            "id": 2,
            "sigla": "NE",
            "nome": "Nordeste"
          }
        }
      }
    }
  }
]
```

### penal.json (Penal v2)

Array de dispositivos penais. Cada entrada é **um dispositivo** (artigo/caput, parágrafo, inciso ou alínea),
cobrindo o Código Penal completo (Parte Geral e Parte Especial), a Lei de Contravenções Penais e 35+ leis
especiais (milhares de dispositivos).

```json
{
  "codigo": "121.2.I",
  "artigo": 121,
  "paragrafo": 2,
  "inciso": "I",
  "alinea": null,
  "nivel": "inciso",
  "ordem": 1234,
  "parte": "Parte Especial",
  "titulo": "Título I – Dos Crimes contra a Pessoa",
  "capitulo": "Capítulo I – Dos Crimes contra a Vida",
  "descricao": "Homicídio qualificado - mediante paga ou promessa de recompensa",
  "textoCompleto": "mediante paga ou promessa de recompensa, ou por outro motivo torpe;",
  "tipo": "crime",
  "legislacao": "CP",
  "legislacaoNome": "Código Penal",
  "penaMin": "Reclusão, de 12 a 30 anos",
  "penaMax": "",
  "codigoFormatado": "Art. 121, § 2º, I do CP",
  "fonte": "https://www.planalto.gov.br/ccivil_03/decreto-lei/del2848compilado.htm",
  "dataAtualizacao": "outubro/2026",
  "hashConteudo": "<sha256>",
  "idUnico": "CP:121.2.I"
}
```

**Campos:**

| Campo | Descrição |
|-------|-----------|
| `codigo` | Código do dispositivo dentro da legislação (ver convenções abaixo) |
| `artigo` / `paragrafo` / `inciso` / `alinea` | Componentes do código (`paragrafo`, `inciso`, `alinea` são `null` quando não se aplicam) |
| `nivel` | Granularidade: `artigo` (caput), `paragrafo`, `inciso` ou `alinea` |
| `ordem` | Posição global no seed (CP primeiro em ordem de artigo, depois leis especiais). A API ordena por este campo |
| `parte` / `titulo` / `capitulo` | Estrutura do texto legal (opcionais; vazios em leis sem essa divisão) |
| `tipo` | `crime`, `contravencao`, `disposicao` (não incriminador: Parte Geral, definições, imunidades, causas de aumento) ou `revogado` (mantido para compatibilidade) |
| `legislacao` | `CP`, `LCP`, `ECA`, `CTB`, `CDC` ou o nome da lei (`Lei 11.343/2006`, `Lei 8.137/1990`, `Lei 9.605/98`, ...) |
| `legislacaoNome` | Nome por extenso da legislação |
| `descricao` / `textoCompleto` | Rubrica curta e texto integral do dispositivo |
| `penaMin` / `penaMax` | Pena (texto livre; vazios em dispositivos não incriminadores) |
| `codigoFormatado` | Forma de exibição (`Art. 121, § 2º, I do CP`) |
| `fonte` / `dataAtualizacao` / `hashConteudo` | Rastreabilidade: URL do texto compilado no Planalto, data da versão e SHA256 do conteúdo |
| `idUnico` | **Chave do upsert.** `PREFIXO:CODIGO` (ex. `CP:121`, `DRG:33`, `OTE:1.I`). Sempre explícito no seed |

**Convenções de `codigo`:**

| Código | Significa |
|--------|-----------|
| `121` | Art. 121 (caput) |
| `121-A` | Art. 121-A (artigo acrescentado por lei posterior) |
| `121.2` | Art. 121, § 2º |
| `163.1` | Art. 163, parágrafo único (parágrafo único é sempre `.1`) |
| `121.I` | Art. 121, inciso I (inciso direto do caput) |
| `121.2.I` | Art. 121, § 2º, inciso I |
| `121.2.VII.a` | Art. 121, § 2º, inciso VII, alínea a |

**Prefixos de `idUnico`:** `CP`, `LCP`, `DRG` (Lei 11.343/2006), `ECA`, `CTB`, `AMB` (Lei 9.605/98), `CDC`,
`LVD` (Lei 9.613/98), `MDP` (Lei 11.340/2006), `DES` (Lei 10.826/2003), `HED` (Lei 8.072/1990), `OTE` (Lei 8.137/1990),
`RAC` (Lei 7.716/1989), `TOR` (Lei 9.455/1997), `ORC` (Lei 12.850/2013), `ABU` (Lei 13.869/2019), `TER` (Lei 13.260/2016),
`GEN` (Lei 2.889/1956), `IDO` (Lei 10.741/2003), `SFN` (Lei 7.492/1986), `FAL` (Lei 11.101/2005), `PCD` (Lei 13.146/2015),
`DEF` (Lei 7.853/1989), `TRA` (Lei 9.434/1997), `EPO` (Lei 1.521/1951), `SOL` (Lei 6.766/1979), `SOF` (Lei 9.609/1998),
`BIO` (Lei 11.105/2005), `INT` (Lei 9.296/1996), `HIV` (Lei 12.984/2014), `PRE` (Decreto-Lei 201/1967), `DIS` (Lei 9.029/1995),
`ELE` (Lei 4.737/1965), `PIN` (Lei 9.279/1996), `CVM` (Lei 6.385/1976), `HBO` (Lei 14.344/2022).
O mapa `legislacao → prefixo` vive em `seedPenal` (`internal/bootstrap/migrations.go`).

**Como o seed é gerado:** a partir dos textos **compilados** publicados pelo Planalto (`planalto.gov.br/ccivil_03/...`),
parseados dispositivo a dispositivo (artigo → parágrafo → inciso → alínea), com `ordem` atribuída sequencialmente na
ordem do texto e `hashConteudo` calculado sobre `legislacao:codigo:textoCompleto`. Dispositivos revogados são mantidos
com `tipo: "revogado"` para não quebrar integrações que já referenciavam o `idUnico`.
O gerador (download, parser, descrições curadas e build) está versionado em [`tools/penal-seed`](../tools/penal-seed/README.md).

**Como é carregado:** `seedPenal` faz **upsert por `idUnico`** (insere o que não existe, atualiza o que mudou),
preenchendo `busca` (texto normalizado) e `hashConteudo` quando ausentes. A migration `010_penal_v2_expansao`
roda o seed e, em seguida, **remove** da collection `penal_artigos` qualquer documento cujo `idUnico` não exista
mais no arquivo (guarda de segurança: aborta sem apagar nada se o arquivo tiver menos de 1000 entradas).
Após alterar o seed, incremente `penalCacheVersion` em `internal/http/handlers/penal.go` para invalidar o cache Redis.

## Migrations

O sistema mantém um registro das migrations executadas na collection `migrations`. 
As seeds só serão executadas uma vez. Para re-executar:

1. Remova o documento da migration da collection `migrations`
2. Ou limpe as collections `estados`, `municipios` e `penal_artigos`
3. Reinicie a aplicação

## Fonte dos Dados

Os dados geográficos são baseados nas APIs públicas do IBGE:
- https://servicodados.ibge.gov.br/api/v1/localidades/estados
- https://servicodados.ibge.gov.br/api/v1/localidades/municipios

Os dispositivos penais são extraídos dos textos compilados do Planalto (campo `fonte` em cada entrada):
- https://www.planalto.gov.br/ccivil_03/decreto-lei/del2848compilado.htm (Código Penal)
- https://www.planalto.gov.br/ccivil_03/decreto-lei/del3688.htm (Lei de Contravenções Penais)
- Demais leis especiais: `planalto.gov.br/ccivil_03/leis/...`

