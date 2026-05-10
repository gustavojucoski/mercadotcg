package cardmarket_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/cardmarket"
)

// TestSearch_PikachuEx busca listagens do Pikachu ex no Cardmarket.
// O Cardmarket usa Cloudflare, então o scraper retorna lista vazia sem erro
// — o handler injeta preços sintéticos via pokemontcg.io como fallback.
func TestSearch_PikachuEx(t *testing.T) {
	c := cardmarket.New(20 * time.Second)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:       "Pikachu ex",
		ExternalID: "https://www.cardmarket.com/en/Pokemon/Products/Singles/Scarlet-and-Violet-Black-Star-Promos/Pikachu-ex-276-217",
	})
	if err != nil {
		t.Fatalf("erro inesperado (Cloudflare deve retornar lista vazia, não erro): %v", err)
	}
	// Cardmarket usa Cloudflare → espera-se lista vazia sem erro.
	// Se por algum motivo retornar resultados (scraping funcionou), valida-os.
	fmt.Printf("Resultados: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  rawcond:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.RawCondition)
	}

	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.Condition] {
			t.Errorf("condição duplicada: %s", r.Condition)
		}
		seen[r.Condition] = true
	}
}

// TestSearch_ViaFlareSolverr testa o scraper com FlareSolverr real.
// Requer FLARESOLVERR_URL no ambiente; caso contrário o teste é pulado.
func TestSearch_ViaFlareSolverr(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado — pulando teste live via FlareSolverr")
	}

	c := cardmarket.NewWithFlareSolverr(90*time.Second, fsURL)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:       "Pikachu ex",
		ExternalID: "https://www.cardmarket.com/en/Pokemon/Products/Singles/Scarlet-and-Violet-Black-Star-Promos/Pikachu-ex-276-217",
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	fmt.Printf("Resultados via FlareSolverr: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  rawcond:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.RawCondition)
	}
	if len(results) == 0 {
		t.Error("nenhum resultado — FlareSolverr não conseguiu bypass?")
	}
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.Condition] {
			t.Errorf("condição duplicada: %s", r.Condition)
		}
		seen[r.Condition] = true
	}
}

// TestSearch_SemExternalID verifica que sem ExternalID a busca retorna lista vazia.
func TestSearch_SemExternalID(t *testing.T) {
	c := cardmarket.New(5 * time.Second)
	results, err := c.Search(context.Background(), scraper.Query{
		Name: "Pikachu ex",
	})
	if err != nil {
		t.Fatalf("esperava lista vazia, mas houve erro: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("esperava 0 resultados, got %d", len(results))
	}
}
