package cardmarket_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		Name:       "Charizard ex",
		ExternalID: "https://www.cardmarket.com/en/Pokemon/Products/Singles/Obsidian-Flames/Charizard-ex-V1-OBF125",
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

// TestDebugFlareSolverrHTML inspeciona o HTML retornado via FlareSolverr para ajustar seletores.
func TestDebugFlareSolverrHTML(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	// Usa a API interna do FlareSolverr diretamente para obter o HTML bruto.
	reqBody := `{"cmd":"request.get","url":"https://www.cardmarket.com/en/Pokemon/Products/Singles/Obsidian-Flames/Charizard-ex-V1-OBF125","maxTimeout":60000}`
	resp, err := http.Post(fsURL+"/v1", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("FlareSolverr request falhou: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("FlareSolverr status: %d, body size: %d", resp.StatusCode, len(body))

	var result struct {
		Status   string `json:"status"`
		Solution struct {
			Status   int    `json:"status"`
			Response string `json:"response"`
		} `json:"solution"`
	}
	if err := json.NewDecoder(strings.NewReader(string(body))).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	html := result.Solution.Response
	t.Logf("Solution status: %d | HTML size: %d bytes", result.Solution.Status, len(html))

	// Procura padrões chave no HTML
	patterns := []string{
		"article-row",
		"article-condition",
		"product-comments",
		"price-container",
		"color-primary",
		"single-card",
		"table-articles",
		"articleCount",
		"data-cm-condition",
		"condition",
		"class=\"row",
		"class=\"col",
		"data-price",
		"EUR",
		"class=\"price",
	}
	for _, pat := range patterns {
		idx := strings.Index(html, pat)
		if idx >= 0 {
			start := idx - 60
			if start < 0 {
				start = 0
			}
			end := idx + 250
			if end > len(html) {
				end = len(html)
			}
			t.Logf("✅ '%s' @ %d:\n%s\n", pat, idx, html[start:end])
		} else {
			t.Logf("❌ '%s' não encontrado", pat)
		}
	}

	// Primeiros 3000 chars do HTML para contexto
	preview := html
	if len(preview) > 3000 {
		preview = preview[:3000]
	}
	t.Logf("=== INÍCIO DO HTML ===\n%s", preview)
}

// TestSearch_ViaSetListing testa a resolução da URL via listagem do set no Cardmarket.
// Requer FLARESOLVERR_URL no ambiente; caso contrário o teste é pulado.
func TestSearch_ViaSetListing(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	c := cardmarket.NewWithFlareSolverr(90*time.Second, fsURL)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:    "Mega Dragonite ex",
		SetName: "Ascended Heroes",
		// ExternalID vazio — deve resolver via set listing
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	fmt.Printf("Resultados via set listing: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  rawcond:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.RawCondition)
	}
	if len(results) == 0 {
		t.Error("nenhum resultado — resolveFromSetListing não encontrou a carta?")
	}
}

// TestDebugSetListing dumpa todos os slugs encontrados no set listing do Cardmarket.
// Útil para entender a estrutura dos slugs (números, V-numbers) para um set específico.
// Requer FLARESOLVERR_URL no ambiente; caso contrário o teste é pulado.
func TestDebugSetListing(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	setName := "Ascended Heroes"
	setSlug := "Ascended-Heroes"
	linkPrefix := "/en/Pokemon/Products/Singles/" + setSlug + "/"

	reqBody := fmt.Sprintf(`{"cmd":"request.get","url":"https://www.cardmarket.com/en/Pokemon/Products/Singles/%s","maxTimeout":60000}`, setSlug)
	resp, err := http.Post(fsURL+"/v1", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("FlareSolverr request falhou: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status   string `json:"status"`
		Solution struct {
			Status   int    `json:"status"`
			Response string `json:"response"`
		} `json:"solution"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	html := result.Solution.Response
	t.Logf("Set listing %q | solution status: %d | HTML size: %d bytes", setName, result.Solution.Status, len(html))

	// Coleta todos os slugs únicos do set listing
	seen := map[string]bool{}
	var slugs []string
	idx := 0
	for {
		pos := strings.Index(html[idx:], linkPrefix)
		if pos < 0 {
			break
		}
		pos += idx
		start := pos + len(linkPrefix)
		end := strings.IndexByte(html[start:], '"')
		if end < 0 {
			break
		}
		slug := html[start : start+end]
		if slug != "" && !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
		idx = pos + 1
	}

	t.Logf("Total de slugs únicos encontrados: %d", len(slugs))

	// Filtra os cards de interesse
	keywords := []string{"pikachu", "dragonite", "charizard"}
	for _, slug := range slugs {
		lower := strings.ToLower(slug)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				t.Logf("  %s", slug)
				break
			}
		}
	}

	// Imprime os primeiros 50 e últimos 10 slugs para contexto
	t.Log("=== Primeiros 50 slugs ===")
	for i, s := range slugs {
		if i >= 50 {
			break
		}
		t.Logf("  [%03d] %s", i+1, s)
	}
	if len(slugs) > 50 {
		t.Log("=== Últimos 10 slugs ===")
		for _, s := range slugs[len(slugs)-10:] {
			t.Logf("  %s", s)
		}
	}
}

// TestDebugSetListingSearch testa se o Cardmarket suporta filtro por nome no set listing.
// Útil para descobrir se existe URL que retorna todas as versões de uma carta específica.
func TestDebugSetListingSearch(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	setSlug := "Ascended-Heroes"
	linkPrefix := "/en/Pokemon/Products/Singles/" + setSlug + "/"

	// Tenta diferentes parâmetros de busca para ver qual funciona
	candidates := []struct {
		label string
		url   string
	}{
		{"sem filtro (página 2)", "https://www.cardmarket.com/en/Pokemon/Products/Singles/" + setSlug + "?site=2"},
		{"searchString Pikachu ex", "https://www.cardmarket.com/en/Pokemon/Products/Singles/" + setSlug + "?searchString=Pikachu+ex"},
		{"name Pikachu ex", "https://www.cardmarket.com/en/Pokemon/Products/Singles/" + setSlug + "?name=Pikachu+ex"},
	}

	for _, c := range candidates {
		t.Run(c.label, func(t *testing.T) {
			reqBody := fmt.Sprintf(`{"cmd":"request.get","url":%q,"maxTimeout":60000}`, c.url)
			resp, err := http.Post(fsURL+"/v1", "application/json", strings.NewReader(reqBody))
			if err != nil {
				t.Fatalf("FlareSolverr: %v", err)
			}
			defer resp.Body.Close()

			var result struct {
				Solution struct {
					URL      string `json:"url"`
					Response string `json:"response"`
				} `json:"solution"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			html := result.Solution.Response

			t.Logf("URL final: %s | HTML: %d bytes", result.Solution.URL, len(html))

			seen := map[string]bool{}
			var pikachus []string
			idx := 0
			for {
				pos := strings.Index(html[idx:], linkPrefix)
				if pos < 0 {
					break
				}
				pos += idx
				start := pos + len(linkPrefix)
				end := strings.IndexByte(html[start:], '"')
				if end < 0 {
					break
				}
				slug := html[start : start+end]
				if !seen[slug] {
					seen[slug] = true
					lower := strings.ToLower(slug)
					if strings.Contains(lower, "pikachu") {
						pikachus = append(pikachus, slug)
					}
				}
				idx = pos + 1
			}

			t.Logf("Total slugs únicos: %d | slugs Pikachu: %d", len(seen), len(pikachus))
			for _, s := range pikachus {
				t.Logf("  PIKACHU: %s", s)
			}
		})
	}
}

// TestSearch_PikachuExSIR testa a busca do Pikachu ex SIR (276/217) do set ASC.
// SIR cards usam o slug {CardName}-{SetCode}{Number} sem prefixo V.
// URL confirmada: https://www.cardmarket.com/en/Pokemon/Products/Singles/Ascended-Heroes/Pikachu-ex-ASC276
// Requer FLARESOLVERR_URL no ambiente; caso contrário o teste é pulado.
func TestSearch_PikachuExSIR(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	c := cardmarket.NewWithFlareSolverr(90*time.Second, fsURL)
	results, err := c.Search(context.Background(), scraper.Query{
		Name:            "Pikachu ex",
		Number:          "276",
		SetCode:         "ASC",
		SetName:         "Ascended Heroes",
		SetPrintedTotal: 217,
		// ExternalID vazio — deve construir URL via SIR shortcut
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	fmt.Printf("Resultados Pikachu ex SIR: %d\n", len(results))
	for _, r := range results {
		fmt.Printf("[%s] %s → %s  cond:%s  rawcond:%s\n",
			r.Currency, r.Title, r.Price, r.Condition, r.RawCondition)
	}
	if len(results) == 0 {
		t.Error("nenhum resultado — SIR URL construída não funcionou?")
	}
}

// TestDebugVNumbersInHTML verifica se versões V2/V3 de cartas aparecem em qualquer parte do HTML
// do set listing (não apenas nos links da listagem). Útil para validar a heurística
// "se V2/V3 está no HTML, V1 é a versão comum; senão, V1 é a única versão".
func TestDebugVNumbersInHTML(t *testing.T) {
	fsURL := os.Getenv("FLARESOLVERR_URL")
	if fsURL == "" {
		t.Skip("FLARESOLVERR_URL não configurado")
	}

	setSlug := "Ascended-Heroes"
	reqBody := fmt.Sprintf(`{"cmd":"request.get","url":"https://www.cardmarket.com/en/Pokemon/Products/Singles/%s","maxTimeout":60000}`, setSlug)
	resp, err := http.Post(fsURL+"/v1", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("FlareSolverr request falhou: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Solution struct {
			Response string `json:"response"`
		} `json:"solution"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	html := strings.ToLower(result.Solution.Response)
	t.Logf("HTML size: %d bytes", len(html))

	checks := []struct {
		card    string
		pattern string
	}{
		{"pikachu-ex V2", "pikachu-ex-v2-"},
		{"pikachu-ex V3", "pikachu-ex-v3-"},
		{"pikachu-ex V4", "pikachu-ex-v4-"},
		{"mega-dragonite-ex V2", "mega-dragonite-ex-v2-"},
		{"mega-dragonite-ex V3", "mega-dragonite-ex-v3-"},
		{"mega-charizard-y-ex V2", "mega-charizard-y-ex-v2-"},
	}

	for _, c := range checks {
		if strings.Contains(html, c.pattern) {
			t.Logf("✅ '%s' encontrado no HTML → múltiplas versões existem", c.card)
		} else {
			t.Logf("❌ '%s' NÃO encontrado → V1 é provavelmente a única versão", c.card)
		}
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
