# Gerador do seed de artigos penais (`seeds/penal.json`)

Pipeline reproduzível que transforma os **textos compilados oficiais do Planalto** no seed consumido pela migration `seedPenal` (upsert por `idUnico`).

```bash
cd tools/penal-seed
python3 fetch.py        # baixa os HTMLs compilados do Planalto para ./src
python3 html2txt.py     # converte para texto (remove trechos revogados em <strike>/line-through)
python3 build.py        # gera ../../seeds/penal.json e imprime estatísticas
```

Sem dependências externas (Python 3.10+, stdlib).

## Arquivos

| Arquivo | Função |
|---|---|
| `fetch.py` | Lista de leis e URLs oficiais; baixa os HTMLs. |
| `html2txt.py` | HTML → texto, uma linha por parágrafo. Remove texto riscado (dispositivos com redação anterior/revogada). Trata encoding (windows-1252, UTF-16 duplo). |
| `parse.py` | Parser genérico do texto compilado: artigos, parágrafos (`§`/parágrafo único), incisos, alíneas, rubricas, linhas "Pena –", títulos/capítulos, dispositivos revogados/vetados; remove anotações "(Incluído pela Lei…)" |
| `overrides.py` | Descrições curadas (nomen juris) para dispositivos sem rubrica oficial, `tipo` e pena forçados por `idUnico`. |
| `build.py` | Configuração das leis (código curto, faixa de artigos, nome, fonte), regras de `tipo`/`nivel`/pena, geração das entradas, validações e escrita do JSON. |

## Convenções do seed

- `idUnico` = `CODIGO:codigo` (ex.: `CP:121`, `CP:121.2.I`, `DRG:33.4`, `AMB:38.1`). Nunca muda: é a chave do upsert e a referência usada pelos tenants.
- `codigo`: `121` (caput), `121-A`, `121.2` (§ 2º), `163.1` (parágrafo único), `121.2.I` (inciso), `121.2.VII.a` (alínea).
- `nivel`: `artigo` | `paragrafo` | `inciso` | `alinea`.
- `tipo`: `crime` | `contravencao` | `disposicao` (não incriminador: Parte Geral, imunidades, definições, ação penal) | `revogado` (mantido para consulta histórica).
- `penaMin`/`penaMax`: `"Reclusão, de 6 a 20 anos"` / `"e multa"`. Parágrafos "nas mesmas penas incorre…" e incisos herdam a pena do dispositivo que os encabeça.
- `ordem`: ordem global de exibição (CP primeiro, em ordem de artigo; depois as leis especiais).
- `parte`/`titulo`/`capitulo`: estrutura do diploma legal, quando existente.
- `dataAtualizacao`: mês/ano da captura dos textos (constante `DATA_ATUALIZACAO` em `build.py`).

## Atualizando a base

1. Rode os três passos acima.
2. Confira a saída de `build.py`: total, contagem por tipo/nível e a lista "idUnicos do seed anterior ausentes no novo" (deve ser vazia, salvo remoção intencional).
3. Ajuste `overrides.py` para novos artigos sem rubrica.
4. Suba a versão do cache em `internal/http/handlers/penal.go` (`penalCacheVersion`) e adicione uma migration que chame `seedPenal`.
