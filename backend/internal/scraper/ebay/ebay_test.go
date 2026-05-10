package ebay_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
)

// TestSearch_PikachuEx busca listagens do Pikachu ex no eBay via Browse API.
// Requer EBAY_CLIENT_ID e EBAY_CLIENT_SECRET no ambiente; caso contrário o
// teste é pulado.
func TestSearch_PikachuEx(t *testing.T) {
	clientID := os.Getenv("EBAY_CLIENT_ID")
	certID := os.Getenv("EBAY_CLIENT_SECRET")
	if clientID == "" || certID == "" {
		t.Skip("EBAY_CLIENT_ID / EBAY_CLIENT_SECRET não configurados — pulando teste live")
	}

	c := ebay.New(20*time.Second, clientID, certID)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:   "Pikachu ex",
		Number: "276",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("nenhum resultado retornado")
	}
	fmt.Printf("Total de resultados: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  url:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.URL)
	}
}

// TestSearch_SemCredenciais verifica que ErrNotConfigured é retornado
// quando as credenciais estão ausentes.
func TestSearch_SemCredenciais(t *testing.T) {
	c := ebay.New(5*time.Second, "", "")
	_, err := c.Search(context.Background(), scraper.Query{
		Name:  "Pikachu ex",
		Limit: 5,
	})
	if err == nil {
		t.Fatal("esperava erro, mas não houve")
	}
	if !errors.Is(err, scraper.ErrNotConfigured) {
		t.Fatalf("erro inesperado: %v (esperava ErrNotConfigured)", err)
	}
}
