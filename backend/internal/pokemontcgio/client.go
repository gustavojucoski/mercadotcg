// Package pokemontcgio é um cliente para a API pública https://api.pokemontcg.io/v2.
//
// Uso principal: resolver nome da carta e product ID do TCGPlayer a partir
// do ptcgoCode do set (ex.: "ASC") + número da carta (ex.: "276"), sem
// precisar pré-popular nenhuma tabela local.
//
// O product ID do TCGPlayer é extraído seguindo o redirect em
// prices.pokemontcg.io → tcgplayer.pxf.io?u=https://tcgplayer.com/product/{id}.
//
// Resultados ficam em cache por 24h para respeitar o rate limit da API
// (1 000 req/dia sem key, 20 000/dia com key).
package pokemontcgio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const apiBase = "https://api.pokemontcg.io/v2"

// ErrNotFound é retornado quando a carta não existe na pokemontcg.io.
var ErrNotFound = errors.New("pokemontcgio: card not found")

// TCGPriceRange contém os pontos de preço de uma impressão no TCGPlayer.
// Todos os campos são ponteiros porque podem ser nulos na resposta da API.
type TCGPriceRange struct {
	Low    *float64 `json:"low"`
	Mid    *float64 `json:"mid"`
	High   *float64 `json:"high"`
	Market *float64 `json:"market"`
}

// CardmarketPriceRange contém os pontos de preço do Cardmarket (EUR).
// Todos os campos são ponteiros porque podem ser nulos na resposta da API.
type CardmarketPriceRange struct {
	AverageSellPrice *float64 `json:"averageSellPrice"`
	LowPrice         *float64 `json:"lowPrice"`
	TrendPrice       *float64 `json:"trendPrice"`
	Avg1             *float64 `json:"avg1"`
	Avg7             *float64 `json:"avg7"`
	Avg30            *float64 `json:"avg30"`
}

// CardInfo contém os dados resolvidos de uma carta.
type CardInfo struct {
	ID               string                    // pokemontcg.io card ID, ex.: "me2pt5-276"
	Name             string                    // nome em inglês, ex.: "Pikachu ex"
	Number           string                    // número no set, ex.: "276"
	SetCode          string                    // ptcgoCode, ex.: "ASC"
	SetName          string                    // nome do set, ex.: "Ascended Heroes"
	SetPrintedTotal  int                       // total de cartas numeradas no set (sem secret rares), ex.: 217
	TCGPlayerID      string                    // product ID no TCGPlayer; vazio se redirect falhou
	TCGPlayerURL     string                    // URL canonical do TCGPlayer (prices.pokemontcg.io)
	TCGPlayerPrices  map[string]TCGPriceRange  // preços por impressão: "holofoil", "normal", "reverseHolofoil"
	CardmarketURL    string                    // URL canonical do Cardmarket (prices.pokemontcg.io)
	CardmarketPrices *CardmarketPriceRange     // preços em EUR; nil se pokemontcg.io não retornou
}

type cacheEntry struct {
	info      CardInfo
	expiresAt time.Time
}

// Client é o cliente HTTP da pokemontcg.io com cache em memória.
type Client struct {
	http       *http.Client
	noRedirect *http.Client
	apiKey     string

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// New cria um Client. apiKey é opcional mas eleva o rate limit de 1k para 20k req/dia.
func New(timeout time.Duration, apiKey string) *Client {
	noRedir := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{
		http:       &http.Client{Timeout: timeout},
		noRedirect: noRedir,
		apiKey:     apiKey,
		cache:      make(map[string]cacheEntry),
	}
}

// FindCard resolve uma carta pelo ptcgoCode do set + número da carta.
// Também resolve o TCGPlayer product ID via redirect (não-fatal se falhar).
// Resultados ficam em cache por 24h.
func (c *Client) FindCard(ctx context.Context, ptcgoCode, number string) (CardInfo, error) {
	key := strings.ToUpper(ptcgoCode) + ":" + number

	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expiresAt) {
		c.mu.Unlock()
		return e.info, nil
	}
	c.mu.Unlock()

	q := fmt.Sprintf("set.ptcgoCode:%s number:%s", ptcgoCode, number)
	target := fmt.Sprintf("%s/cards?q=%s&pageSize=1&select=id,name,number,set,tcgplayer,cardmarket",
		apiBase, url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return CardInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return CardInfo{}, fmt.Errorf("pokemontcgio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CardInfo{}, fmt.Errorf("pokemontcgio: status %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Number string `json:"number"`
			Set    struct {
				PtcgoCode    string `json:"ptcgoCode"`
				Name         string `json:"name"`
				PrintedTotal int    `json:"printedTotal"`
			} `json:"set"`
			TCGPlayer struct {
				URL    string                   `json:"url"`
				Prices map[string]TCGPriceRange `json:"prices"`
			} `json:"tcgplayer"`
			Cardmarket struct {
				URL    string                `json:"url"`
				Prices *CardmarketPriceRange `json:"prices"`
			} `json:"cardmarket"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return CardInfo{}, fmt.Errorf("pokemontcgio: decode: %w", err)
	}
	if len(body.Data) == 0 {
		return CardInfo{}, ErrNotFound
	}

	d := body.Data[0]
	info := CardInfo{
		ID:               d.ID,
		Name:             d.Name,
		Number:           d.Number,
		SetCode:          d.Set.PtcgoCode,
		SetName:          d.Set.Name,
		SetPrintedTotal:  d.Set.PrintedTotal,
		TCGPlayerURL:     d.TCGPlayer.URL,
		TCGPlayerPrices:  d.TCGPlayer.Prices,
		CardmarketURL:    d.Cardmarket.URL,
		CardmarketPrices: d.Cardmarket.Prices,
	}

	// Tenta resolver o product ID via redirect — best-effort (não-fatal se a API rate-limitar).
	// Estratégia 1: usa o tcgplayer.url do pokemontcg.io (prices.pokemontcg.io/tcgplayer/{id}).
	// Estratégia 2 (fallback): Scrydex mapeia todos os cards mesmo sem entry no pokemontcg.io;
	//   GET scrydex.com/pokemon/cards/{id}/purchase?variant=holofoil → 302 → tcgplayer.com/product/{id}
	redirectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if d.TCGPlayer.URL != "" {
		if tcgID, err := c.resolveTCGPlayerID(redirectCtx, d.TCGPlayer.URL); err == nil {
			info.TCGPlayerID = tcgID
		}
	}
	if info.TCGPlayerID == "" {
		scrydexPurchaseURL := fmt.Sprintf("https://scrydex.com/pokemon/cards/%s/purchase?variant=holofoil", d.ID)
		if tcgID, err := c.resolveTCGPlayerID(redirectCtx, scrydexPurchaseURL); err == nil {
			info.TCGPlayerID = tcgID
		}
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{info: info, expiresAt: time.Now().Add(24 * time.Hour)}
	c.mu.Unlock()

	return info, nil
}

// resolveTCGPlayerID segue o redirect em prices.pokemontcg.io e extrai o product ID.
//
// Fluxo:
//
//	HEAD https://prices.pokemontcg.io/tcgplayer/me2pt5-276
//	→ Location: https://tcgplayer.pxf.io/scrydex?u=https://tcgplayer.com/product/676088
//	→ extrai "676088"
func (c *Client) resolveTCGPlayerID(ctx context.Context, pricesURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, pricesURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.noRedirect.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no Location header from %s", pricesURL)
	}

	// Location: https://tcgplayer.pxf.io/scrydex?u=https://tcgplayer.com/product/676088
	u, err := url.Parse(loc)
	if err != nil {
		return "", err
	}

	productURL := u.Query().Get("u")
	if productURL == "" {
		productURL = loc
	}

	pu, err := url.Parse(productURL)
	if err != nil {
		return "", err
	}

	// Extrai o ID do segmento após /product/ no path.
	parts := strings.Split(pu.Path, "/")
	for i, p := range parts {
		if p == "product" && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1], nil
		}
	}

	return "", fmt.Errorf("no product ID in %s", productURL)
}
