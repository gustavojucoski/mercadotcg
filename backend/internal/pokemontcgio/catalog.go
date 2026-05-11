package pokemontcgio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SetInfo contém os dados de um set retornados pela API de catálogo.
type SetInfo struct {
	ID           string
	Name         string
	Series       string
	PrintedTotal int
	Total        int
	ReleaseDate  string
	LogoURL      string
	SymbolURL    string
}

// CatalogCard contém os dados de uma carta para importação de catálogo.
type CatalogCard struct {
	ID        string
	Name      string
	Number    string
	Rarity    string
	Supertype string
	Subtypes  []string
	Types     []string
	HP        string
	Artist    string
	SmallURL  string
	LargeURL  string
}

// ListSets retorna todos os sets disponíveis na pokemontcg.io,
// ordenados por data de lançamento.
func (c *Client) ListSets(ctx context.Context) ([]SetInfo, error) {
	const pageSize = 250
	target := fmt.Sprintf("%s/sets?pageSize=%d&orderBy=releaseDate", apiBase, pageSize)

	resp, err := c.requestWithRetry(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("pokemontcgio list sets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pokemontcgio list sets: status %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Series       string `json:"series"`
			PrintedTotal int    `json:"printedTotal"`
			Total        int    `json:"total"`
			ReleaseDate  string `json:"releaseDate"`
			Images       struct {
				Logo   string `json:"logo"`
				Symbol string `json:"symbol"`
			} `json:"images"`
		} `json:"data"`
		TotalCount int `json:"totalCount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("pokemontcgio list sets decode: %w", err)
	}

	sets := make([]SetInfo, 0, len(body.Data))
	for _, d := range body.Data {
		sets = append(sets, SetInfo{
			ID:           d.ID,
			Name:         d.Name,
			Series:       d.Series,
			PrintedTotal: d.PrintedTotal,
			Total:        d.Total,
			ReleaseDate:  d.ReleaseDate,
			LogoURL:      d.Images.Logo,
			SymbolURL:    d.Images.Symbol,
		})
	}
	return sets, nil
}

// ListCardsBySet retorna todas as cartas de um set, tratando paginação automaticamente.
func (c *Client) ListCardsBySet(ctx context.Context, setID string) ([]CatalogCard, error) {
	const pageSize = 250
	var all []CatalogCard
	page := 1

	for {
		q := url.QueryEscape(fmt.Sprintf("set.id:%s", setID))
		target := fmt.Sprintf("%s/cards?q=%s&pageSize=%d&page=%d", apiBase, q, pageSize, page)

		resp, err := c.requestWithRetry(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("pokemontcgio list cards (set=%s page=%d): %w", setID, page, err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("pokemontcgio list cards: status %d", resp.StatusCode)
		}

		var body struct {
			Data []struct {
				ID        string   `json:"id"`
				Name      string   `json:"name"`
				Number    string   `json:"number"`
				Rarity    string   `json:"rarity"`
				Supertype string   `json:"supertype"`
				Subtypes  []string `json:"subtypes"`
				Types     []string `json:"types"`
				HP        string   `json:"hp"`
				Artist    string   `json:"artist"`
				Images    struct {
					Small string `json:"small"`
					Large string `json:"large"`
				} `json:"images"`
			} `json:"data"`
			TotalCount int `json:"totalCount"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("pokemontcgio list cards decode: %w", err)
		}
		resp.Body.Close()

		for _, d := range body.Data {
			all = append(all, CatalogCard{
				ID:        d.ID,
				Name:      d.Name,
				Number:    d.Number,
				Rarity:    d.Rarity,
				Supertype: d.Supertype,
				Subtypes:  d.Subtypes,
				Types:     d.Types,
				HP:        d.HP,
				Artist:    d.Artist,
				SmallURL:  d.Images.Small,
				LargeURL:  d.Images.Large,
			})
		}

		// Sai quando a página retornou menos itens que o pageSize (última página)
		// ou quando já buscamos todos conforme totalCount.
		if len(body.Data) < pageSize || len(all) >= body.TotalCount {
			break
		}
		page++
	}
	return all, nil
}

// requestWithRetry executa um GET com retry em caso de rate limiting (429).
// Respeita o header Retry-After quando presente, com backoff exponencial como fallback.
func (c *Client) requestWithRetry(ctx context.Context, rawURL string) (*http.Response, error) {
	const maxAttempts = 5
	backoff := 2 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if c.apiKey != "" {
			req.Header.Set("X-Api-Key", c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		// 429: fechar body e aguardar antes de tentar novamente.
		resp.Body.Close()

		if attempt == maxAttempts-1 {
			return nil, fmt.Errorf("rate limited após %d tentativas: %s", maxAttempts, rawURL)
		}

		wait := backoff
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}

	// Nunca alcançado, mas o compilador exige retorno explícito.
	return nil, fmt.Errorf("pokemontcgio: requestWithRetry: unreachable")
}
