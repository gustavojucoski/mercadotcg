package ligapokemon_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
)

func TestSearch_PikachuEx(t *testing.T) {
	c := ligapokemon.New(15 * time.Second)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:    "Pikachu ex",
		Number:  "276",
		SetCode: "ASC",
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	fmt.Printf("Total de listagens: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → R$ %s  stock:%d  lang:%s  extra:%s  url:%s\n",
			r.Condition, r.Title, r.Price, r.Stock, r.Language, r.RawCondition, r.URL)
	}
}

// TestDebugRawHTML inspeciona o HTML retornado pelo site.
func TestDebugRawHTML(t *testing.T) {
	// Testa duas URLs: a da busca e a do card direto com nome completo
	targets := []string{
		"https://www.ligapokemon.com.br/?view=cards/card&card=Pikachu+ex&ed=ASC&num=276",
		"https://www.ligapokemon.com.br/?view=cards/card&card=Pikachu+ex+%28276%2F217%29&ed=ASC&num=276",
		"https://www.ligapokemon.com.br/?view=cards/search&card=Pikachu+ex",
	}

	for _, target := range targets {
		t.Logf("\n===== URL: %s =====", target)

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.5,en;q=0.3")
		req.Header.Set("Accept-Encoding", "identity")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("request falhou: %v", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(body)

		t.Logf("Status: %d | Tamanho: %d bytes", resp.StatusCode, len(body))

		// Procura variáveis JS conhecidas
		for _, v := range []string{"cards_stock", "cards_stores", "cards_editions", "var cards"} {
			if idx := strings.Index(bodyStr, v); idx >= 0 {
				end := idx + 300
				if end > len(bodyStr) {
					end = len(bodyStr)
				}
				t.Logf("✅ '%s' em pos %d: ...%s...", v, idx, bodyStr[idx:end])
			} else {
				t.Logf("❌ '%s' não encontrado", v)
			}
		}

		// Encontra todos os "var " no body para ver quais variáveis JS existem
		varCount := 0
		pos := 0
		for varCount < 20 {
			idx := strings.Index(bodyStr[pos:], "var ")
			if idx < 0 {
				break
			}
			abs := pos + idx
			end := abs + 60
			if end > len(bodyStr) {
				end = len(bodyStr)
			}
			line := strings.Split(bodyStr[abs:end], "\n")[0]
			t.Logf("  var[%d]: %s", varCount, strings.TrimSpace(line))
			pos = abs + 4
			varCount++
		}

		// Links de card na página (para ver se há resultados de busca)
		linkCount := 0
		searchStr := "view=cards/card"
		pos = 0
		for linkCount < 5 {
			idx := strings.Index(bodyStr[pos:], searchStr)
			if idx < 0 {
				break
			}
			abs := pos + idx
			end := abs + 150
			if end > len(bodyStr) {
				end = len(bodyStr)
			}
			t.Logf("  link[%d]: ...%s...", linkCount, bodyStr[abs:end])
			pos = abs + len(searchStr)
			linkCount++
		}
		if linkCount == 0 {
			t.Logf("  Nenhum link 'view=cards/card' encontrado")
		}
	}
}
