package tcgplayer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/tcgplayer"
)

// TestSearch_PikachuEx busca o Pikachu ex 276/217 (Ascended Heroes) pelo
// product ID do TCGplayer. URL original:
// https://www.tcgplayer.com/product/676088/Pokemon-ME%20Ascended%20Heroes-Pikachu%20ex%20276%20217
func TestSearch_PikachuEx(t *testing.T) {
	c := tcgplayer.New(20 * time.Second)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:       "Pikachu ex",
		Number:     "276",
		SetCode:    "ASC",
		ExternalID: "676088",
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("nenhum resultado retornado")
	}
	fmt.Printf("Total de resultados: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s %s  url:%s\n",
			r.Currency, r.Title, r.Price, r.RawCondition, r.URL)
	}
}

// TestSearch_SemExternalID verifica que ErrNotConfigured é retornado
// quando não há ExternalID (TCGplayer não tem API de busca pública).
func TestSearch_SemExternalID(t *testing.T) {
	c := tcgplayer.New(5 * time.Second)
	_, err := c.Search(context.Background(), scraper.Query{
		Name:  "Pikachu ex",
		Limit: 5,
	})
	if err == nil {
		t.Fatal("esperava erro, mas não houve")
	}
	if err.Error() != scraper.ErrNotConfigured.Error() {
		t.Fatalf("erro inesperado: %v (esperava ErrNotConfigured)", err)
	}
}
