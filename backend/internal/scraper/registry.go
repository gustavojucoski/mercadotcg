// internal/scraper/registry.go
//
// Registry implementa prioridade primária/fallback e circuit breaker simples
// por source lógico (pricing.Source). O design é intencionalmente mínimo:
//
//   - Circuit breaker sem janela de tempo: conta apenas falhas consecutivas.
//   - Reset imediato em qualquer sucesso.
//   - Sem half-open: quando o circuito está aberto o source é simplesmente
//     pulado. Adequado para MVP onde o problema típico é "pokewallet.io down".
//
// Como scraper.Query não carrega um campo pricing.Source, o Registry NÃO
// implementa a interface scraper.Source. Em vez disso expõe SearchForSource,
// que o cmd/api usa diretamente no lugar do fan-out flat anterior.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// Priority determina a ordem de tentativa dentro de um logical source.
// Valores menores são tentados primeiro.
type Priority int

const (
	PrimarySource  Priority = 1
	FallbackSource Priority = 2
)

// Registration associa uma implementação de Source a um logical source,
// com prioridade e limiar de falhas consecutivas para abertura do circuito.
type Registration struct {
	Source           Source
	Priority         Priority
	FailureThreshold int // abrir circuito após N falhas consecutivas
	id               int // chave interna única para o circuit breaker — atribuída por Register
}

// Registry despacha buscas para implementações concretas de um logical source,
// respeitando prioridade e aplicando circuit breaker por implementação.
type Registry struct {
	// sources mapeia pricing.Source → lista ordenada por Priority.
	sources map[pricing.Source][]Registration

	// failures conta falhas consecutivas por Registration.id.
	// Keyed por ID interno (não por Source.Name()) para que duas implementações
	// com o mesmo Name() — ex.: primary e fallback de pokewallet — mantenham
	// contadores independentes.
	failures map[int]int

	nextID int // contador monotônico para gerar IDs únicos de Registration

	mu sync.RWMutex
}

// NewRegistry cria um Registry vazio.
func NewRegistry() *Registry {
	return &Registry{
		sources:  make(map[pricing.Source][]Registration),
		failures: make(map[int]int),
	}
}

// Register adiciona uma implementação concreta para um logical source.
// Pode ser chamado múltiplas vezes para o mesmo logical source (ex.: primário + fallback).
// failThreshold é o número de falhas consecutivas antes de abrir o circuito;
// use 0 para nunca abrir (não recomendado em produção).
func (r *Registry) Register(logical pricing.Source, impl Source, priority Priority, failThreshold int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	reg := Registration{
		Source:           impl,
		Priority:         priority,
		FailureThreshold: failThreshold,
		id:               r.nextID,
	}
	r.sources[logical] = append(r.sources[logical], reg)

	// Manter lista ordenada por Priority para que Search não precise reordenar.
	sort.Slice(r.sources[logical], func(i, j int) bool {
		return r.sources[logical][i].Priority < r.sources[logical][j].Priority
	})
}

// SearchForSource executa a busca para o logical source pedido, respeitando
// prioridade e circuit breaker. O comportamento é:
//
//  1. Itera implementações em ordem de prioridade.
//  2. Se o circuito estiver aberto para uma implementação, ela é pulada.
//  3. ErrNotConfigured conta como falha (source não pronto) — aciona fallback.
//  4. Sucesso (mesmo com slice vazio) zera o contador de falhas.
//  5. Se todas as implementações falharem ou estiverem com circuito aberto,
//     retorna ErrAllSourcesFailed.
func (r *Registry) SearchForSource(ctx context.Context, logical pricing.Source, q Query) ([]Result, error) {
	r.mu.RLock()
	regs, ok := r.sources[logical]
	r.mu.RUnlock()

	if !ok || len(regs) == 0 {
		return nil, fmt.Errorf("scraper registry: nenhuma implementação registrada para %q", logical)
	}

	var lastErr error

	for _, reg := range regs {
		name := string(reg.Source.Name())

		// Verificar se o circuito está aberto para esta implementação.
		if reg.FailureThreshold > 0 && r.consecutiveFailures(reg.id) >= reg.FailureThreshold {
			log.Warn().
				Str("source", name).
				Int("failures", r.consecutiveFailures(reg.id)).
				Int("threshold", reg.FailureThreshold).
				Str("logical", string(logical)).
				Msg("scraper registry: circuito aberto, pulando source")
			continue
		}

		results, err := reg.Source.Search(ctx, q)

		if err != nil {
			// ErrNotConfigured: source não tem credenciais — aciona fallback sem
			// incrementar contador (não é uma falha transitória).
			if errors.Is(err, ErrNotConfigured) {
				log.Warn().
					Str("source", name).
					Str("logical", string(logical)).
					Msg("scraper registry: source não configurado, tentando fallback")
				lastErr = err
				continue
			}

			// Falha real: incrementa contador e tenta fallback.
			newCount := r.recordFailure(reg.id)
			log.Warn().
				Err(err).
				Str("source", name).
				Str("logical", string(logical)).
				Int("consecutive_failures", newCount).
				Msg("scraper registry: falha na fonte primária, tentando fallback")

			if reg.FailureThreshold > 0 && newCount >= reg.FailureThreshold {
				log.Error().
					Str("source", name).
					Int("failures", newCount).
					Msg("scraper registry: limiar atingido, circuito aberto")
			}

			lastErr = err
			continue
		}

		// Sucesso — resetar contador de falhas.
		r.resetFailures(reg.id)
		return results, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("scraper registry: %w: %w", ErrAllSourcesFailed, lastErr)
	}
	return nil, ErrAllSourcesFailed
}

// consecutiveFailures lê o contador de falhas de forma segura para leitura.
func (r *Registry) consecutiveFailures(id int) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.failures[id]
}

// recordFailure incrementa e retorna o novo contador de falhas consecutivas.
func (r *Registry) recordFailure(id int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[id]++
	return r.failures[id]
}

// resetFailures zera o contador de falhas de uma Registration após um sucesso.
func (r *Registry) resetFailures(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failures, id)
}

// ─── SourceAdapter ────────────────────────────────────────────────────────────

// SourceAdapter envolve um Registry + logical source em um scraper.Source,
// permitindo que o Registry seja integrado no fan-out plano do ExternalHandler
// sem modificar o handler. Name() retorna o logical source; Search() despacha
// para SearchForSource, mapeando ErrAllSourcesFailed para lista vazia + log.
type SourceAdapter struct {
	registry *Registry
	logical  pricing.Source
}

// ForSource retorna um scraper.Source que representa o logical source no fan-out.
// Registre o SourceAdapter no lugar das implementações diretas em main.go.
func (r *Registry) ForSource(logical pricing.Source) Source {
	return &SourceAdapter{registry: r, logical: logical}
}

// Name implementa scraper.Source.
func (a *SourceAdapter) Name() pricing.Source { return a.logical }

// Search implementa scraper.Source. Converte ErrAllSourcesFailed em resultado
// vazio com log — para que o fan-out do handler continue normalmente.
func (a *SourceAdapter) Search(ctx context.Context, q Query) ([]Result, error) {
	results, err := a.registry.SearchForSource(ctx, a.logical, q)
	if err != nil {
		if errors.Is(err, ErrAllSourcesFailed) {
			// Retorna erro para que MeasureSearch o registre como campo Error
			// na SourceResult — visível no JSON de resposta do admin.
			return nil, err
		}
		return nil, err
	}
	return results, nil
}
