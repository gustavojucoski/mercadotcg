package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gustavojucoski/mercadotcg/backend/internal/handler"
	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ebay"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/ligapokemon"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper/tcgplayer"
)

// externalSearchResp espelha o JSON devolvido pelo endpoint.
type externalSearchResp struct {
	Card    *struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Number  string `json:"number"`
		SetCode string `json:"set_code"`
		SetName string `json:"set_name"`
	} `json:"card"`
	Query struct {
		Name       string `json:"Name"`
		Number     string `json:"Number"`
		SetCode    string `json:"SetCode"`
		ExternalID string `json:"ExternalID"`
		Limit      int    `json:"Limit"`
	} `json:"query"`
	Sources []struct {
		Source     string `json:"source"`
		DurationMs int64  `json:"duration_ms"`
		Error      string `json:"error"`
		Results    []struct {
			Title     string `json:"title"`
			Price     string `json:"price"`
			Currency  string `json:"currency"`
			Condition string `json:"condition"`
			URL       string `json:"url"`
		} `json:"results"`
	} `json:"sources"`
}

func newRouter(t *testing.T) *chi.Mux {
	t.Helper()
	catalog := pokemontcgio.New(15*time.Second, "")
	h := handler.NewExternalHandler(
		ligapokemon.New(20*time.Second),
		tcgplayer.New(20*time.Second),
		ebay.New(20*time.Second, "", ""), // sem credenciais → ErrNotConfigured
	).WithCatalog(catalog)

	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// TestExternalSearch_NumberAndSet verifica o fluxo principal:
// passar apenas number+set deve resolver nome e IDs automaticamente.
func TestExternalSearch_NumberAndSet(t *testing.T) {
	r := newRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/external-search?number=276&set=ASC&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var resp externalSearchResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Catálogo deve ter resolvido a carta.
	if resp.Card == nil {
		t.Error("card == nil — pokemontcgio lookup falhou")
	} else {
		t.Logf("card resolvido: id=%s name=%q set=%s", resp.Card.ID, resp.Card.Name, resp.Card.SetCode)
		if resp.Card.Name == "" {
			t.Error("card.name vazio")
		}
		if resp.Card.SetCode != "ASC" {
			t.Errorf("set_code: got %q, want ASC", resp.Card.SetCode)
		}
	}

	// Query deve ter o nome preenchido pelo catalog.
	if resp.Query.Name == "" {
		t.Error("query.Name vazio — catalog não enriqueceu a query")
	}

	// Verifica fontes.
	for _, src := range resp.Sources {
		fmt.Printf("[%s] %dms  err=%q  resultados=%d\n",
			src.Source, src.DurationMs, src.Error, len(src.Results))
		for _, res := range src.Results {
			fmt.Printf("    %s %s %s — %s\n", res.Currency, res.Price, res.Condition, res.Title)
		}
	}

	// LigaPokemon deve ter resultados (não requer credencial).
	ligaOK := false
	tcgOK := false
	for _, src := range resp.Sources {
		if src.Source == "ligapokemon" {
			if src.Error != "" {
				t.Errorf("ligapokemon error: %s", src.Error)
			}
			if len(src.Results) == 0 {
				t.Error("ligapokemon: nenhum resultado")
			}
			ligaOK = true
		}
		if src.Source == "tcgplayer" {
			if src.Error != "" {
				t.Errorf("tcgplayer error: %s", src.Error)
			}
			if len(src.Results) == 0 {
				t.Error("tcgplayer: nenhum resultado")
			}
			tcgOK = true
		}
	}
	if !ligaOK {
		t.Error("fonte ligapokemon ausente na resposta")
	}
	if !tcgOK {
		t.Error("fonte tcgplayer ausente na resposta")
	}
}

// TestExternalSearch_SemParametros verifica rejeição de request sem parâmetros.
func TestExternalSearch_SemParametros(t *testing.T) {
	r := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/external-search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, got %d", w.Code)
	}
}

// TestExternalSearch_ApenasName funciona sem number+set (sem resolução de catálogo).
func TestExternalSearch_ApenasName(t *testing.T) {
	r := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/external-search?name=Pikachu+ex&limit=3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var resp externalSearchResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Sem number+set, card deve ser nil (sem resolução de catálogo).
	if resp.Card != nil {
		t.Logf("card resolvido mesmo sem number+set: %v", resp.Card)
	}
	for _, src := range resp.Sources {
		fmt.Printf("[%s] %dms  err=%q  resultados=%d\n",
			src.Source, src.DurationMs, src.Error, len(src.Results))
	}
}
