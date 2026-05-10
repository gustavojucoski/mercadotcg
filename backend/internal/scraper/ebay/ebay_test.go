package ebay_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
)

// TestSearch_MegaDragoniteEx busca vendas eBay do Mega Dragonite ex via Scrydex.
// Usa o pokemontcg.io card ID como ExternalID; sem credenciais, sem API key.
func TestSearch_MegaDragoniteEx(t *testing.T) {
	c := ebay.New(20 * time.Second)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:       "Mega Dragonite ex",
		ExternalID: "me2pt5-290", // pokemontcg.io card ID
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("nenhum resultado retornado")
	}
	fmt.Printf("Total de resultados: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  rawcond:%s  url:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.RawCondition, r.URL)
	}

	// Verifica que não há grade duplicada.
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.RawCondition] {
			t.Errorf("rawCondition duplicado: %s", r.RawCondition)
		}
		seen[r.RawCondition] = true
	}
}

// TestSearch_SemExternalID verifica que sem ExternalID a busca retorna lista vazia.
func TestSearch_SemExternalID(t *testing.T) {
	c := ebay.New(5 * time.Second)
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
