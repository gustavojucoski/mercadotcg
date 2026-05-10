// Command import-catalog popula card_sets + cards a partir da Pokemon TCG API
// (https://pokemontcg.io/), que é grátis e cobre todas as cartas oficiais
// do TCG (~30k entries).
//
// Uso:
//   import-catalog              # importa todos os sets + cards
//   import-catalog --set sv8    # importa só um set específico
//   import-catalog --recent 5   # importa só os 5 sets mais recentes
//
// Idempotente: cards/sets já existentes são pulados (UNIQUE conflict tratado).
//
// Variáveis de ambiente:
//   POKEMON_TCG_API_KEY  (opcional) — aumenta rate limit de 1k/dia para 20k/dia.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/config"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

const apiBase = "https://api.pokemontcg.io/v2"

func main() {
	setCode := flag.String("set", "", "código de um set específico (ex.: sv8). Se vazio, importa todos.")
	recent := flag.Int("recent", 0, "se > 0, importa só os N sets mais recentes.")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail("config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fail("connect postgres: %v", err)
	}
	defer pool.Close()

	repo := postgres.NewCardRepo(pool)
	client := newAPIClient(cfg.PokemonTCGAPIKey)

	fmt.Println("==> Importando catálogo Pokemon TCG")

	sets, err := client.fetchSets(ctx)
	if err != nil {
		fail("fetch sets: %v", err)
	}
	fmt.Printf("    %d sets encontrados na API\n", len(sets))

	// Filtros: --set ou --recent.
	if *setCode != "" {
		sets = filterByCode(sets, *setCode)
		if len(sets) == 0 {
			fail("nenhum set encontrado com code=%q", *setCode)
		}
	} else if *recent > 0 {
		sets = mostRecent(sets, *recent)
	}

	imported, skipped, cardsTotal := 0, 0, 0
	for i, s := range sets {
		fmt.Printf("[%d/%d] %s — %s\n", i+1, len(sets), s.ID, s.Name)

		dbSet, err := upsertSet(ctx, repo, s)
		if err != nil {
			fmt.Printf("    erro no set %s: %v (pulando cards desse set)\n", s.ID, err)
			continue
		}
		if errors.Is(err, postgres.ErrAlreadyExists) {
			skipped++
		} else {
			imported++
		}

		cards, err := client.fetchCards(ctx, s.ID)
		if err != nil {
			fmt.Printf("    erro nos cards: %v\n", err)
			continue
		}

		var inserted, conflicts int
		for _, c := range cards {
			if err := upsertCard(ctx, repo, dbSet.ID, c); err != nil {
				if errors.Is(err, postgres.ErrAlreadyExists) {
					conflicts++
					continue
				}
				fmt.Printf("    erro em %s: %v\n", c.Number, err)
				continue
			}
			inserted++
		}
		fmt.Printf("    %d cards novos, %d já existiam\n", inserted, conflicts)
		cardsTotal += inserted

		// Rate limit conservador: pausa entre sets pra ser educado com a API.
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println()
	fmt.Printf("==> Concluído: %d sets novos, %d sets já existiam, %d cards novos\n",
		imported, skipped, cardsTotal)
}

// ----------------------------------------------------------------------------
// Cliente HTTP da API
// ----------------------------------------------------------------------------

type apiClient struct {
	http   *http.Client
	apiKey string
}

func newAPIClient(apiKey string) *apiClient {
	return &apiClient{
		http:   &http.Client{Timeout: 30 * time.Second},
		apiKey: apiKey,
	}
}

func (a *apiClient) request(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if a.apiKey != "" {
		req.Header.Set("X-Api-Key", a.apiKey)
	}
	return a.http.Do(req)
}

// apiSet espelha o JSON dos sets na Pokemon TCG API.
type apiSet struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Series       string `json:"series"`
	PrintedTotal int    `json:"printedTotal"`
	Total        int    `json:"total"`
	ReleaseDate  string `json:"releaseDate"` // formato "YYYY/MM/DD"
	Images       struct {
		Logo   string `json:"logo"`
		Symbol string `json:"symbol"`
	} `json:"images"`
}

func (a *apiClient) fetchSets(ctx context.Context) ([]apiSet, error) {
	var allSets []apiSet
	page := 1
	const pageSize = 250

	for {
		target := fmt.Sprintf("%s/sets?page=%d&pageSize=%d&orderBy=releaseDate", apiBase, page, pageSize)
		resp, err := a.request(ctx, target)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("sets status %d", resp.StatusCode)
		}
		var body struct {
			Data       []apiSet `json:"data"`
			TotalCount int      `json:"totalCount"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode sets: %w", err)
		}
		allSets = append(allSets, body.Data...)
		if len(body.Data) < pageSize {
			break
		}
		page++
	}
	return allSets, nil
}

// apiCard espelha cards. Só os campos usados.
type apiCard struct {
	ID        string   `json:"id"` // "sv8-199"
	Name      string   `json:"name"`
	Number    string   `json:"number"`
	Rarity    string   `json:"rarity"`
	Supertype string   `json:"supertype"`
	Subtypes  []string `json:"subtypes"`
	Types     []string `json:"types"`
	HP        string   `json:"hp"` // string mesmo (algumas cartas não têm HP)
	Artist    string   `json:"artist"`
	Images    struct {
		Small string `json:"small"`
		Large string `json:"large"`
	} `json:"images"`
}

func (a *apiClient) fetchCards(ctx context.Context, setID string) ([]apiCard, error) {
	var all []apiCard
	page := 1
	const pageSize = 250

	for {
		q := url.QueryEscape(fmt.Sprintf("set.id:%s", setID))
		target := fmt.Sprintf("%s/cards?q=%s&page=%d&pageSize=%d", apiBase, q, page, pageSize)
		resp, err := a.request(ctx, target)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("cards status %d", resp.StatusCode)
		}
		var body struct {
			Data []apiCard `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode cards: %w", err)
		}
		all = append(all, body.Data...)
		if len(body.Data) < pageSize {
			break
		}
		page++
	}
	return all, nil
}

// ----------------------------------------------------------------------------
// Persistência
// ----------------------------------------------------------------------------

func upsertSet(ctx context.Context, repo *postgres.CardRepo, s apiSet) (card.Set, error) {
	existing, err := repo.GetSetByCode(ctx, s.ID)
	if err == nil {
		return existing, postgres.ErrAlreadyExists
	}
	if !errors.Is(err, postgres.ErrNotFound) {
		return card.Set{}, err
	}

	releaseDate := parseAPIDate(s.ReleaseDate)
	dbSet := card.Set{
		Code:        s.ID,
		Name:        s.Name,
		Series:      s.Series,
		Language:    card.LanguageEnglish, // Pokemon TCG API só tem inglês
		ReleaseDate: releaseDate,
		TotalCards:  s.Total,
		ImageURL:    s.Images.Logo,
	}
	if err := repo.CreateSet(ctx, &dbSet); err != nil {
		return card.Set{}, err
	}
	return dbSet, nil
}

func upsertCard(ctx context.Context, repo *postgres.CardRepo, setID uuid.UUID, c apiCard) error {
	// Construímos number como "199/191" pra match humano.
	number := c.Number
	dbCard := card.Card{
		SetID:         setID,
		Number:        number,
		Name:          c.Name,
		Rarity:        c.Rarity,
		Supertype:     c.Supertype,
		Subtypes:      c.Subtypes,
		Types:         c.Types,
		HP:            atoiOrZero(c.HP),
		Illustrator:   c.Artist,
		ImageSmallURL: c.Images.Small,
		ImageLargeURL: c.Images.Large,
		ExternalIDs: map[string]string{
			"pokemon_tcg_io": c.ID,
		},
	}
	if err := repo.CreateCard(ctx, &dbCard); err != nil {
		return err
	}

	// Cria pelo menos a variante "normal" pra cada carta. Variantes
	// específicas (Master Ball, Reverse Holo etc.) ficam para o usuário ou
	// scraper detectarem caso a caso.
	v := card.Variant{
		CardID: dbCard.ID,
		Finish: card.FinishNormal,
	}
	if err := repo.CreateVariant(ctx, &v); err != nil && !errors.Is(err, postgres.ErrAlreadyExists) {
		return err
	}

	// Heurística: se a raridade indica holo/reverse, cria também essas variantes.
	rarityUpper := strings.ToUpper(c.Rarity)
	if strings.Contains(rarityUpper, "HOLO") && !strings.Contains(rarityUpper, "REVERSE") {
		holo := card.Variant{CardID: dbCard.ID, Finish: card.FinishHolo}
		_ = repo.CreateVariant(ctx, &holo) // ignorando erro de duplicata
	}
	if strings.Contains(rarityUpper, "REVERSE") {
		rev := card.Variant{CardID: dbCard.ID, Finish: card.FinishReverseHolo}
		_ = repo.CreateVariant(ctx, &rev)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func filterByCode(sets []apiSet, code string) []apiSet {
	var out []apiSet
	for _, s := range sets {
		if s.ID == code {
			out = append(out, s)
		}
	}
	return out
}

func mostRecent(sets []apiSet, n int) []apiSet {
	sort.SliceStable(sets, func(i, j int) bool {
		return sets[i].ReleaseDate > sets[j].ReleaseDate
	})
	if n > len(sets) {
		n = len(sets)
	}
	return sets[:n]
}

func parseAPIDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006/01/02", s)
	if err != nil {
		return nil
	}
	return &t
}

func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "import-catalog: "+format+"\n", args...)
	os.Exit(1)
}
