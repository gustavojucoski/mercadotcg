// Package scraper define o contrato comum entre as fontes externas de preço
// (LigaPokemon, TCGplayer, eBay). Cada implementação mora num subpacote.
//
// O endpoint GET /api/v1/external-search faz fan-out paralelo a todas as
// fontes registradas e agrega o resultado. Falha numa fonte não quebra o
// resto — basta marcar Error no SourceResult.
package scraper

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// ErrNotConfigured sinaliza que a fonte não tem credenciais/configuração
// suficiente pra rodar (ex.: TCGplayer sem API key). É erro esperado, não
// uma falha — handlers tratam como "fonte indisponível" e seguem o fan-out.
var ErrNotConfigured = errors.New("scraper: source não configurada")

// ErrAllSourcesFailed é retornado pelo Registry quando todas as implementações
// registradas para um logical source falharam (circuito aberto ou erro real).
var ErrAllSourcesFailed = errors.New("scraper: todas as fontes falharam para este source")

// Query é o critério de busca. Pelo menos um campo precisa estar preenchido;
// cada fonte decide quais usa (Liga combina nome+número; eBay só nome).
type Query struct {
	Name            string // nome da carta — "Charizard ex"
	Number          string // número canônico — "199/191" ou só "199"
	SetCode         string // código do set — "sv8" (nem toda fonte aceita)
	SetName         string // nome do set em inglês — "Ascended Heroes" (Cardmarket resolver)
	SetPrintedTotal int    // total de cartas numeradas do set (sem secret rares) — ex.: 217
	ExternalID      string // ID nativo da fonte (ex: product ID do TCGPlayer — "676088")
	Limit           int    // máximo de resultados desejados (cada fonte respeita ou ignora)
}

// Result é uma observação bruta extraída de uma fonte.
//
// Ainda NÃO normalizamos para BRL aqui — o handler decide se converte ou
// devolve crú. A vantagem de não converter no scraper: respostas mais rápidas
// (não dependem do forex.Service na crítica de cada hit).
type Result struct {
	Title         string          `json:"title"`               // "Charizard ex - SV8 199/191 - Hyper Rare"
	URL           string          `json:"url"`                 // link direto pro produto
	ImageURL      string          `json:"image_url,omitempty"`
	Condition     string          `json:"condition,omitempty"` // raw da fonte; pode não casar com nosso enum
	Language      string          `json:"language,omitempty"`  // "Português", "English", ...
	Price         decimal.Decimal `json:"price"`
	Currency      pricing.Currency `json:"currency"`
	Stock         int             `json:"stock,omitempty"`     // quantos vendedores/unidades, quando a fonte expõe
	Kind          pricing.Kind    `json:"kind,omitempty"`      // sale ou listing (default listing)
	ExternalID    string          `json:"external_id,omitempty"`
	RawCondition  string          `json:"raw_condition,omitempty"`
}

// SourceResult é o que cada Source devolve numa busca: lista de resultados
// + metadata (tempo, erro). Nunca devolve nil — sempre estrutura válida pro
// JSON ficar limpo no front.
type SourceResult struct {
	Source     pricing.Source `json:"source"`
	DurationMS int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Results    []Result       `json:"results"`
}

// Source é o contrato que cada fonte implementa.
type Source interface {
	// Name devolve o identificador (bate com pricing.Source).
	Name() pricing.Source

	// Search executa a busca. Implementações DEVEM respeitar context.Done()
	// (timeout/cancel) e DEVEM retornar []Result vazia + error em caso de
	// falha — nunca panic.
	Search(ctx context.Context, q Query) ([]Result, error)
}

// MeasureSearch chama Source.Search medindo duração e empacotando o resultado
// num SourceResult pronto para JSON. Centraliza tratamento uniforme entre
// fontes (tempo, erro string-friendly).
func MeasureSearch(ctx context.Context, src Source, q Query) SourceResult {
	start := time.Now()
	results, err := src.Search(ctx, q)
	out := SourceResult{
		Source:     src.Name(),
		DurationMS: time.Since(start).Milliseconds(),
		Results:    results,
	}
	if err != nil {
		out.Error = err.Error()
	}
	if out.Results == nil {
		out.Results = []Result{}
	}
	return out
}
