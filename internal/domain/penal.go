package domain

import "time"

// ArtigoPenal representa um dispositivo do Código Penal ou de legislação especial.
//
// Cada documento corresponde a UM dispositivo (artigo/caput, parágrafo, inciso ou alínea),
// identificado de forma única por IdUnico ("PREFIXO:CODIGO", ex: "CP:121.2.I").
type ArtigoPenal struct {
	ID string `json:"id" bson:"_id,omitempty"`
	// Codigo identifica o dispositivo dentro da legislação. Convenções:
	//   "121"          → Art. 121 (caput)
	//   "121-A"        → Art. 121-A (artigo acrescentado por lei posterior)
	//   "121.2"        → Art. 121, § 2º
	//   "163.1"        → Art. 163, parágrafo único (parágrafo único é sempre ".1")
	//   "121.2.I"      → Art. 121, § 2º, inciso I
	//   "121.2.VII.a"  → Art. 121, § 2º, inciso VII, alínea a
	//   "121.I"        → Art. 121, inciso I (inciso direto do caput, sem parágrafo)
	Codigo    string  `json:"codigo" bson:"codigo"`
	Artigo    int     `json:"artigo" bson:"artigo"` // 121
	Paragrafo *int    `json:"paragrafo,omitempty" bson:"paragrafo,omitempty"`
	Inciso    *string `json:"inciso,omitempty" bson:"inciso,omitempty"` // "I", "II", "III"
	Alinea    *string `json:"alinea,omitempty" bson:"alinea,omitempty"` // "a", "b", "c"
	// Nivel indica a granularidade do dispositivo: "artigo" | "paragrafo" | "inciso" | "alinea"
	Nivel string `json:"nivel" bson:"nivel"`
	// Ordem é a posição global do dispositivo no seed (CP primeiro, em ordem de artigo; depois leis especiais).
	// Usada para ordenar listagens de forma estável e fiel ao texto legal.
	Ordem         int    `json:"ordem" bson:"ordem"`
	Descricao     string `json:"descricao" bson:"descricao"`         // "Homicídio simples"
	TextoCompleto string `json:"textoCompleto" bson:"textoCompleto"` // Texto completo do dispositivo
	// Tipo classifica o dispositivo:
	//   "crime"        → tipo penal incriminador (crime)
	//   "contravencao" → contravenção penal (LCP)
	//   "disposicao"   → dispositivo não incriminador (Parte Geral, definições, imunidades, causas de aumento etc)
	//   "revogado"     → dispositivo revogado, mantido para compatibilidade retroativa
	Tipo string `json:"tipo" bson:"tipo"`
	// Estrutura do texto legal (opcional): "Parte Geral"/"Parte Especial", título e capítulo
	Parte           string `json:"parte,omitempty" bson:"parte,omitempty"`       // "Parte Geral", "Parte Especial"
	Titulo          string `json:"titulo,omitempty" bson:"titulo,omitempty"`     // "Título I – Dos Crimes contra a Pessoa"
	Capitulo        string `json:"capitulo,omitempty" bson:"capitulo,omitempty"` // "Capítulo I – Dos Crimes contra a Vida"
	Legislacao      string `json:"legislacao" bson:"legislacao"`                 // "CP", "LCP", "Lei 11.343/2006", "Lei 8.137/1990"
	LegislacaoNome  string `json:"legislacaoNome" bson:"legislacaoNome"`         // "Código Penal", "Lei de Contravenções Penais"
	PenaMin         string `json:"penaMin,omitempty" bson:"penaMin,omitempty"`   // "Reclusão, de 6 a 20 anos"
	PenaMax         string `json:"penaMax,omitempty" bson:"penaMax,omitempty"`   // "e multa"
	CodigoFormatado string `json:"codigoFormatado" bson:"codigoFormatado"`       // "Art. 121, § 1º, I, a) do CP"
	// Para busca e autocomplete
	Busca string `json:"-" bson:"busca"` // Texto normalizado para busca (lowercase, sem acentos)
	// Documentação e rastreabilidade
	Fonte           string    `json:"fonte" bson:"fonte"`                                   // URL ou referência da fonte oficial
	DataAtualizacao string    `json:"dataAtualizacao" bson:"dataAtualizacao"`               // Data da última atualização da fonte oficial
	HashConteudo    string    `json:"hashConteudo,omitempty" bson:"hashConteudo,omitempty"` // SHA256 para detectar alterações
	IdUnico         string    `json:"idUnico" bson:"idUnico"`                               // Identificador único: "PREFIXO:CODIGO" (ex: "CP:121", "DRG:33", "OTE:1.I")
	CreatedAt       time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt" bson:"updatedAt"`
}

// PenalResponse representa a resposta da API para autocomplete
type PenalResponse struct {
	Codigo          string `json:"codigo"`
	CodigoFormatado string `json:"codigoFormatado"`
	Descricao       string `json:"descricao"`
	Tipo            string `json:"tipo"`
	Nivel           string `json:"nivel"` // "artigo" | "paragrafo" | "inciso" | "alinea"
	Legislacao      string `json:"legislacao"`
	LegislacaoNome  string `json:"legislacaoNome"`
	IdUnico         string `json:"idUnico"` // Identificador único: "LEGISLACAO:CODIGO" para diferenciar artigos com mesmo código em legislações diferentes
}
