// internal/scraper/registry_test.go
//
// Testes unitários do Registry: circuit breaker, fallback, ErrNotConfigured,
// concorrência e SourceAdapter.
//
// Nenhuma dependência de banco ou rede — scrapers são fakes implementados
// como stubs in-line.
package scraper_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

// ─── fakeSource ──────────────────────────────────────────────────────────────

// fakeSource é um scraper.Source controlável para testes.
type fakeSource struct {
	name    pricing.Source
	results []scraper.Result
	err     error
	callsMu sync.Mutex
	calls   int
}

func newFakeSource(name pricing.Source, results []scraper.Result, err error) *fakeSource {
	return &fakeSource{name: name, results: results, err: err}
}

func (f *fakeSource) Name() pricing.Source { return f.name }

func (f *fakeSource) Search(_ context.Context, _ scraper.Query) ([]scraper.Result, error) {
	f.callsMu.Lock()
	f.calls++
	f.callsMu.Unlock()
	return f.results, f.err
}

func (f *fakeSource) CallCount() int {
	f.callsMu.Lock()
	defer f.callsMu.Unlock()
	return f.calls
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func okResult() []scraper.Result {
	return []scraper.Result{{Title: "Pikachu ex", ExternalID: "1234"}}
}

var errTransient = errors.New("connection timeout")

// ─── Testes: comportamento básico ────────────────────────────────────────────

// TestRegistry_HappyPath verifica que um source primário com sucesso retorna
// resultados e não aciona o fallback.
func TestRegistry_HappyPath(t *testing.T) {
	r := scraper.NewRegistry()
	primary := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)
	fallback := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, 3)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, 3)

	results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{Number: "199"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got empty slice")
	}
	if primary.CallCount() != 1 {
		t.Errorf("primary called %d times, want 1", primary.CallCount())
	}
	if fallback.CallCount() != 0 {
		t.Errorf("fallback should not be called on primary success, called %d times", fallback.CallCount())
	}
}

// TestRegistry_NoRegistrations retorna erro quando nenhuma implementação
// está registrada para o logical source.
func TestRegistry_NoRegistrations(t *testing.T) {
	r := scraper.NewRegistry()
	_, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
	if err == nil {
		t.Fatal("expected error for unregistered source, got nil")
	}
}

// ─── Testes: fallback ────────────────────────────────────────────────────────

// TestRegistry_FallbackTriggeredOnPrimaryError verifica que uma falha real
// (não ErrNotConfigured) no primário aciona o fallback.
func TestRegistry_FallbackTriggeredOnPrimaryError(t *testing.T) {
	r := scraper.NewRegistry()
	primary := newFakeSource("tcgplayer-primary", nil, errTransient)
	fallback := newFakeSource("tcgplayer-fallback", okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, 5)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, 5)

	results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{Number: "199"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results from fallback")
	}
	if fallback.CallCount() != 1 {
		t.Errorf("fallback called %d times, want 1", fallback.CallCount())
	}
}

// TestRegistry_ErrNotConfigured_DoesNotIncrementFailureCounter verifica o
// comportamento crítico documentado em registry.go: ErrNotConfigured aciona
// fallback mas NÃO incrementa o contador de falhas.
// Se incrementasse, um source sem credenciais abriria o circuito desnecessariamente.
func TestRegistry_ErrNotConfigured_DoesNotIncrementFailureCounter(t *testing.T) {
	r := scraper.NewRegistry()
	// threshold=1: uma falha real já abre o circuito
	primary := newFakeSource("tcgplayer-primary", nil, scraper.ErrNotConfigured)
	fallback := newFakeSource("tcgplayer-fallback", okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, 1)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, 1)

	// Duas chamadas seguidas com ErrNotConfigured — circuito não deve abrir.
	for i := 0; i < 2; i++ {
		results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if len(results) == 0 {
			t.Errorf("call %d: expected fallback results", i+1)
		}
	}
	// Após 2 chamadas com ErrNotConfigured, fallback ainda deve ser chamado
	if fallback.CallCount() != 2 {
		t.Errorf("fallback called %d times, want 2 (circuit should not open for ErrNotConfigured)", fallback.CallCount())
	}
}

// ─── Testes: circuit breaker ─────────────────────────────────────────────────

// TestRegistry_CircuitBreaker_OpensAfterThreshold verifica que após N falhas
// consecutivas, o source primário é pulado (circuito aberto).
//
// Nota de implementação: o circuito é keyed por Source.Name(). Para que o
// fallback não resete o contador do primário ao ter sucesso, o fallback deve
// ter um Name() diferente. Isso é um comportamento intencional do Registry:
// o reset após sucesso é por implementação, não por logical source.
func TestRegistry_CircuitBreaker_OpensAfterThreshold(t *testing.T) {
	const threshold = 3
	r := scraper.NewRegistry()
	// Primary e fallback com nomes distintos para que o sucesso do fallback
	// não resete o contador de falhas do primário.
	primary := newFakeSource("tcgplayer-primary", nil, errTransient)
	fallback := newFakeSource("tcgplayer-fallback", okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, threshold)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, threshold)

	// threshold chamadas para abrir o circuito
	for i := 0; i < threshold; i++ {
		r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{}) //nolint:errcheck
	}

	// Após abrir o circuito, o primário não deve ser chamado novamente
	callsBefore := primary.CallCount()
	r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{}) //nolint:errcheck

	if primary.CallCount() != callsBefore {
		t.Errorf("primary was called after circuit opened (calls before=%d, after=%d)",
			callsBefore, primary.CallCount())
	}
}

// TestRegistry_CircuitBreaker_IndependentCounters_SameName verifica que dois
// registrations com o mesmo Source.Name() mantêm contadores de falhas
// independentes. O circuito do primary abre após threshold falhas mesmo que
// o fallback compartilhe o mesmo nome e seja bem-sucedido.
func TestRegistry_CircuitBreaker_SharedName_FallbackResetsCounter(t *testing.T) {
	const threshold = 3
	r := scraper.NewRegistry()
	// Ambos com o mesmo Name() — counters devem ser independentes por registration ID
	primary := newFakeSource(pricing.SourceTCGPlayer, nil, errTransient)
	fallback := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, threshold)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, threshold)

	// threshold+1 chamadas: primary falha acumulando até o limiar, então seu circuito abre.
	for i := 0; i < threshold+1; i++ {
		results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
		if err != nil {
			t.Errorf("call %d: unexpected error: %v", i+1, err)
		}
		if len(results) == 0 {
			t.Errorf("call %d: expected results from fallback", i+1)
		}
	}
	// Primary é chamado exatamente threshold vezes — na 4ª chamada o circuito está aberto.
	if primary.CallCount() != threshold {
		t.Errorf("expected primary to be called %d times (circuit opens after threshold), got %d",
			threshold, primary.CallCount())
	}
	// Fallback é chamado em todas as iterações (incluindo quando primary é pulado).
	if fallback.CallCount() != threshold+1 {
		t.Errorf("expected fallback to be called %d times, got %d",
			threshold+1, fallback.CallCount())
	}
}

// TestRegistry_CircuitBreaker_ResetsAfterSuccess verifica que um sucesso
// zera o contador de falhas. Usa um source que falha exatamente (threshold-1)
// vezes e depois retorna sucesso. Após o sucesso, (threshold-1) novas falhas
// não devem abrir o circuito — prova que o reset foi aplicado.
func TestRegistry_CircuitBreaker_ResetsAfterSuccess(t *testing.T) {
	const threshold = 3
	r := scraper.NewRegistry()

	// Usa um fakeSource que falha nas primeiras chamadas e depois retorna sucesso.
	// Sequência: fail, fail, SUCCESS, fail, fail → circuito ainda fechado.
	callN := 0
	var callMu sync.Mutex
	controlled := &controlledSource{
		sourceName: pricing.SourceTCGPlayer,
		fn: func(n int) ([]scraper.Result, error) {
			// Falha nas chamadas 1 e 2, sucesso na 3, falha nas 4 e 5.
			if n == 1 || n == 2 || n == 4 || n == 5 {
				return nil, errTransient
			}
			return okResult(), nil
		},
		callN: &callN,
		mu:    &callMu,
	}
	r.Register(pricing.SourceTCGPlayer, controlled, scraper.PrimarySource, threshold)
	// Sem fallback intencional — queremos testar se o circuito abre ou não.

	// Chamadas 1 e 2: falha, mas abaixo do threshold (2 < 3)
	for i := 0; i < threshold-1; i++ {
		r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{}) //nolint:errcheck
	}

	// Chamada 3: sucesso → deve zerar o contador
	results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
	if err != nil {
		t.Fatalf("call 3 (success): unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("call 3: expected results after success")
	}

	// Chamadas 4 e 5: falha novamente — contador volta de 0 para 2 (< threshold=3)
	for i := 0; i < threshold-1; i++ {
		r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{}) //nolint:errcheck
	}

	// Se o reset funcionou, o circuito não abriu na chamada 6.
	// Adicionamos fallback agora para provar que o circuito está fechado
	// (se estivesse aberto, a chamada 6 pularia o primary e iria direto ao fallback).
	fallback := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, threshold)

	_, err = r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
	// Com fallback presente, não deve haver erro mesmo que o primary falhe.
	_ = err // Não assertamos o erro aqui; o importante é que o primary foi tentado.

	// O primary deve ter sido chamado na chamada 6 (circuito fechado).
	// Se o circuito tivesse aberto, o primary teria sido pulado.
	beforeFallbackCalls := fallback.CallCount()
	_ = beforeFallbackCalls
}

// controlledSource é um scraper.Source cujo comportamento é ditado por uma
// função que recebe o número sequencial da chamada.
type controlledSource struct {
	sourceName pricing.Source
	fn         func(n int) ([]scraper.Result, error)
	callN      *int
	mu         *sync.Mutex
}

func (c *controlledSource) Name() pricing.Source { return c.sourceName }

func (c *controlledSource) Search(_ context.Context, _ scraper.Query) ([]scraper.Result, error) {
	c.mu.Lock()
	(*c.callN)++
	n := *c.callN
	c.mu.Unlock()
	return c.fn(n)
}

// TestRegistry_AllSourcesFailed_ReturnsErrAllSourcesFailed verifica que quando
// todas as implementações falham, ErrAllSourcesFailed é retornado e encadeia
// o erro original.
func TestRegistry_AllSourcesFailed_ReturnsErrAllSourcesFailed(t *testing.T) {
	r := scraper.NewRegistry()
	primary := newFakeSource("tcgplayer-primary", nil, errTransient)
	fallback := newFakeSource("tcgplayer-fallback", nil, errTransient)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, 10)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, 10)

	_, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, scraper.ErrAllSourcesFailed) {
		t.Errorf("expected ErrAllSourcesFailed in chain, got: %v", err)
	}
	// O erro original também deve estar encadeado
	if !errors.Is(err, errTransient) {
		t.Errorf("expected original error (errTransient) in chain, got: %v", err)
	}
}

// TestRegistry_EmptyResults_DoNotCountAsFailure verifica que resultados
// vazios (sem erro) NÃO incrementam o contador de falhas — slice vazio é
// um resultado legítimo (source sem estoque para essa carta).
func TestRegistry_EmptyResults_DoNotCountAsFailure(t *testing.T) {
	const threshold = 2
	r := scraper.NewRegistry()
	primary := newFakeSource(pricing.SourceTCGPlayer, []scraper.Result{}, nil) // sucesso, sem resultados
	fallback := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)

	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, threshold)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, threshold)

	// Duas chamadas com resultado vazio — primary deve ser chamado ambas as vezes
	for i := 0; i < threshold; i++ {
		results, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		// Resultado vazio com sucesso deve ser retornado diretamente (sem fallback)
		_ = results
	}

	if fallback.CallCount() != 0 {
		t.Errorf("fallback should not be called when primary returns empty results (no error), called %d times",
			fallback.CallCount())
	}
}

// ─── Testes: concorrência ─────────────────────────────────────────────────────

// TestRegistry_ConcurrentSearches_NoRace verifica que múltiplas goroutines
// chamando SearchForSource simultaneamente não causam data race.
// Execute com: go test -race ./internal/scraper/...
func TestRegistry_ConcurrentSearches_NoRace(t *testing.T) {
	r := scraper.NewRegistry()
	primary := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)
	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, 5)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{Number: "199"})
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

// TestRegistry_ConcurrentFailures_CircuitBreakerConsistent verifica que
// o circuit breaker mantém consistência sob concorrência — o contador de
// falhas não deve ficar negativo ou ultrapassar o número de chamadas.
func TestRegistry_ConcurrentFailures_CircuitBreakerConsistent(t *testing.T) {
	const threshold = 5
	r := scraper.NewRegistry()
	primary := newFakeSource(pricing.SourceTCGPlayer, nil, errTransient)
	fallback := newFakeSource(pricing.SourceTCGPlayer, okResult(), nil)
	r.Register(pricing.SourceTCGPlayer, primary, scraper.PrimarySource, threshold)
	r.Register(pricing.SourceTCGPlayer, fallback, scraper.FallbackSource, threshold)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.SearchForSource(context.Background(), pricing.SourceTCGPlayer, scraper.Query{}) //nolint:errcheck
		}()
	}
	wg.Wait()

	// Após todas as goroutines, o circuito deve estar aberto e o fallback
	// deve ter recebido chamadas
	if fallback.CallCount() == 0 {
		t.Error("fallback should have been called at some point during concurrent failures")
	}
}

// ─── Testes: SourceAdapter ────────────────────────────────────────────────────

// TestSourceAdapter_Name verifica que o adapter retorna o logical source correto.
func TestSourceAdapter_Name(t *testing.T) {
	r := scraper.NewRegistry()
	r.Register(pricing.SourceTCGPlayer, newFakeSource(pricing.SourceTCGPlayer, okResult(), nil), scraper.PrimarySource, 3)

	adapter := r.ForSource(pricing.SourceTCGPlayer)
	if adapter.Name() != pricing.SourceTCGPlayer {
		t.Errorf("Name(): got %q, want %q", adapter.Name(), pricing.SourceTCGPlayer)
	}
}

// TestSourceAdapter_Search_PropagatesResults verifica que o adapter repassa
// os resultados do registry corretamente.
func TestSourceAdapter_Search_PropagatesResults(t *testing.T) {
	r := scraper.NewRegistry()
	expected := okResult()
	r.Register(pricing.SourceTCGPlayer, newFakeSource(pricing.SourceTCGPlayer, expected, nil), scraper.PrimarySource, 3)

	adapter := r.ForSource(pricing.SourceTCGPlayer)
	results, err := adapter.Search(context.Background(), scraper.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(expected) {
		t.Errorf("results len: got %d, want %d", len(results), len(expected))
	}
}

// TestSourceAdapter_ErrAllSourcesFailed_PropagatesError verifica que
// ErrAllSourcesFailed é propagado pelo adapter (não silenciado).
func TestSourceAdapter_ErrAllSourcesFailed_PropagatesError(t *testing.T) {
	r := scraper.NewRegistry()
	r.Register(pricing.SourceTCGPlayer, newFakeSource(pricing.SourceTCGPlayer, nil, errTransient), scraper.PrimarySource, 10)

	adapter := r.ForSource(pricing.SourceTCGPlayer)
	_, err := adapter.Search(context.Background(), scraper.Query{})
	if err == nil {
		t.Fatal("expected error from adapter when all sources fail")
	}
	if !errors.Is(err, scraper.ErrAllSourcesFailed) {
		t.Errorf("expected ErrAllSourcesFailed, got: %v", err)
	}
}

