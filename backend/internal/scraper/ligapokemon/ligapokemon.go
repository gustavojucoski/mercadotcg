// Package ligapokemon implementa scraper.Source para a Liga Pokémon
// (https://www.ligapokemon.com.br/) via extração de variáveis JavaScript
// embutidas na página — sem API pública.
//
// Estratégia:
//  1. Busca a página de pesquisa: /?view=cards/search&card=QUERY
//  2. Encontra o primeiro link de card: a[href*='view=cards/card']
//  3. Carrega a página de detalhe do card.
//  4. Extrai as variáveis JS inline com regex:
//     - var cards_stock   = [...]   → listings individuais
//     - var cards_stores  = {...}   → mapa lj_id → nome da loja
//     - var cards_editions = [...] → edições com preços agregados
//  5. Converte cada entrada de cards_stock em scraper.Result.
//
// Mapeamentos descobertos inspecionando o HTML real:
//   qualid:  1=Mint, 2=NM, 3=SP/LP, 4=MP, 5=HP, 6=Damaged
//   idioma:  2=English, 6=Japanese, 8=Portuguese, 11=PT+EN
//   extras:  2=Foil, 3=Reverse Foil, 41=Shattered Holo,
//            43=Master Ball, 47=Pokeball Foil
package ligapokemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

const (
	baseURL   = "https://www.ligapokemon.com.br"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Regex para capturar variáveis JS embutidas na página de detalhe.
var (
	reCardsStock    = regexp.MustCompile(`(?i)var\s+cards_stock\s*=\s*(\[[\s\S]*?\]);`)
	reCardsStores   = regexp.MustCompile(`(?i)var\s+cards_stores\s*=\s*(\{[\s\S]*?\});`)
	reCardsEditions = regexp.MustCompile(`(?i)var\s+cards_editions\s*=\s*(\[[\s\S]*?\]);`)
)

// ─── Structs de deserialização JS ─────────────────────────────────────────────

// stockEntry representa um elemento de cards_stock.
type stockEntry struct {
	LjID       json.Number     `json:"lj_id"`
	Qualid     json.Number     `json:"qualid"`
	Idioma     json.Number     `json:"idioma"`
	Extras     json.Number     `json:"extras"`
	Quantidade json.Number     `json:"quantidade"`
	PrecoFinal json.Number     `json:"precoFinal"`
	Foto       json.RawMessage `json:"foto"` // string path or 0 (number when no photo)
}

// storeInfo representa um valor no mapa cards_stores.
type storeInfo struct {
	LjName string `json:"lj_name"`
	LjURL  string `json:"lj_url"`
}

// editionAggPrice contém os preços agregados de uma edição.
type editionAggPrice struct {
	P string `json:"p"` // mínimo
	M string `json:"m"` // médio
	G string `json:"g"` // máximo
}

// editionEntry representa um elemento de cards_editions.
type editionEntry struct {
	EdCode  string                     `json:"ed_code"`
	EdName  string                     `json:"ed_name"`
	CardNum string                     `json:"card_num"`
	Price   map[string]editionAggPrice `json:"price"` // chave "0" = todos
}

// ─── Client ───────────────────────────────────────────────────────────────────

// Client é o scraper LigaPokemon.
type Client struct {
	http    *http.Client
	baseURL string
}

// New cria o client com timeout total por requisição.
func New(timeout time.Duration) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		baseURL: baseURL,
	}
}

// Name implementa scraper.Source.
func (c *Client) Name() pricing.Source { return pricing.SourceLigaPokemon }

// Search implementa scraper.Source.
//
// Se SetCode estiver preenchido, vai direto na página do card via
//   /?view=cards/card&card=NAME&ed=SETCODE&num=NUMBER
// (fluxo rápido e preciso — sem passo de busca).
//
// Sem SetCode, faz busca primeiro e segue o primeiro link encontrado.
func (c *Client) Search(ctx context.Context, q scraper.Query) ([]scraper.Result, error) {
	if q.Name == "" && q.Number == "" {
		return nil, errors.New("ligapokemon: name ou number obrigatório")
	}

	cardURL, err := c.resolveCardURL(ctx, q)
	if err != nil {
		return []scraper.Result{}, nil //nolint:nilerr // sem card = lista vazia
	}

	// Carrega a página de detalhe.
	detailBody, err := c.fetchBody(ctx, cardURL)
	if err != nil {
		return nil, fmt.Errorf("ligapokemon: detalhe: %w", err)
	}

	// Extrai variáveis JS.
	stock, stores, _, err := extractJSVars(detailBody)
	if err != nil {
		return nil, fmt.Errorf("ligapokemon: extrair JS: %w", err)
	}

	all := convertStock(stock, stores, cardURL)
	return cheapestPerCondition(all), nil
}

// resolveCardURL devolve a URL da página de detalhe do card.
//
// A URL de detalhe exige o parâmetro "card=Nome (NUM/TOTAL)" — o total de
// cartas do set não é conhecido em avanço. Por isso sempre passamos pela
// página de busca, que lista os links já com o formato correto, e filtramos
// pelo número e código da edição quando disponíveis.
func (c *Client) resolveCardURL(ctx context.Context, q scraper.Query) (string, error) {
	queryStr := strings.TrimSpace(q.Name)
	// URL de busca construída manualmente para preservar o slash literal em "cards/search".
	searchURL := c.baseURL + "/?view=cards/search&card=" + url.QueryEscape(queryStr)

	searchBody, err := c.fetchBody(ctx, searchURL)
	if err != nil {
		return "", fmt.Errorf("busca: %w", err)
	}

	return findBestCardLink(searchBody, c.baseURL, q.Number, q.SetCode)
}

// findBestCardLink extrai da página de busca o link de detalhe do card mais
// relevante. Prefere o link que bate com num + ed; cai no primeiro link se
// não houver filtros.
func findBestCardLink(body []byte, base, number, setCode string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	var first, exact string
	doc.Find("a[href*='view=cards/card']").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		// Guarda o primeiro link encontrado como fallback.
		if first == "" {
			first = absoluteURL(base, href)
		}
		// Tenta match exato por num + ed.
		if number != "" && setCode != "" {
			if strings.Contains(href, "num="+number) && strings.Contains(href, "ed="+setCode) {
				if exact == "" {
					exact = absoluteURL(base, href)
				}
			}
		} else if number != "" {
			if strings.Contains(href, "num="+number) && exact == "" {
				exact = absoluteURL(base, href)
			}
		} else if setCode != "" {
			if strings.Contains(href, "ed="+setCode) && exact == "" {
				exact = absoluteURL(base, href)
			}
		}
	})

	if exact != "" {
		return exact, nil
	}
	if first != "" {
		return first, nil
	}
	return "", errors.New("nenhum card encontrado na busca")
}

// ─── HTTP helpers ──────────────────────────────────────────────────────────────

func (c *Client) fetchBody(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.5,en;q=0.3")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("status %d em %s", resp.StatusCode, target)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ler body: %w", err)
	}
	return body, nil
}

// firstCardLink encontra o primeiro href de card na página de resultados de busca.

// ─── Extração de variáveis JS ─────────────────────────────────────────────────

func extractJSVars(body []byte) ([]stockEntry, map[string]storeInfo, []editionEntry, error) {
	bodyStr := string(body)

	// cards_stock
	var stock []stockEntry
	if m := reCardsStock.FindStringSubmatch(bodyStr); m != nil {
		if err := json.Unmarshal([]byte(m[1]), &stock); err != nil {
			// Tenta reparar JS (aspas simples → duplas)
			repaired := repairJS(m[1])
			_ = json.Unmarshal([]byte(repaired), &stock)
		}
	}
	if len(stock) == 0 {
		return nil, nil, nil, errors.New("cards_stock vazio ou não encontrado")
	}

	// cards_stores
	stores := make(map[string]storeInfo)
	if m := reCardsStores.FindStringSubmatch(bodyStr); m != nil {
		if err := json.Unmarshal([]byte(m[1]), &stores); err != nil {
			repaired := repairJS(m[1])
			_ = json.Unmarshal([]byte(repaired), &stores)
		}
	}

	// cards_editions (opcional — não quebra se ausente)
	var editions []editionEntry
	if m := reCardsEditions.FindStringSubmatch(bodyStr); m != nil {
		_ = json.Unmarshal([]byte(m[1]), &editions)
	}

	return stock, stores, editions, nil
}

// repairJS tenta converter JSON-like JS (aspas simples, trailing commas) para JSON válido.
// Heurística simples — suficiente para o formato que a Liga usa.
func repairJS(s string) string {
	// Troca aspas simples por duplas (cuidado: pode quebrar strings com apóstrofo).
	s = strings.ReplaceAll(s, `\'`, `__APOS__`)
	s = strings.ReplaceAll(s, `'`, `"`)
	s = strings.ReplaceAll(s, `__APOS__`, `'`)
	return s
}

// ─── Conversão de stock entries ───────────────────────────────────────────────

func convertStock(stock []stockEntry, stores map[string]storeInfo, cardURL string) []scraper.Result {
	results := make([]scraper.Result, 0, len(stock))

	for _, s := range stock {
		priceStr := s.PrecoFinal.String()
		price, err := parseBRLNumber(priceStr)
		if err != nil || price.IsZero() {
			continue
		}

		qualid := toInt(s.Qualid.String())
		condition := mapQualid(qualid)
		rawCond := rawQualid(qualid)

		idioma := toInt(s.Idioma.String())
		language := mapIdioma(idioma)

		extras := toInt(s.Extras.String())
		extrasLabel := mapExtras(extras)

		ljID := s.LjID.String()
		storeName := ""
		storeURL := cardURL
		if info, ok := stores[ljID]; ok {
			storeName = info.LjName
			if info.LjURL != "" {
				storeURL = absoluteURL(baseURL, info.LjURL)
			}
		}

		title := buildTitle(storeName, rawCond, extrasLabel, language)

		imgURL := ""
		var fotoStr string
		if len(s.Foto) > 0 && json.Unmarshal(s.Foto, &fotoStr) == nil && fotoStr != "" {
			imgURL = absoluteURL(baseURL, fotoStr)
		}

		qty := toInt(s.Quantidade.String())

		results = append(results, scraper.Result{
			Title:        title,
			URL:          storeURL,
			ImageURL:     imgURL,
			Price:        price,
			Currency:     pricing.CurrencyBRL,
			Kind:         pricing.KindListing,
			Condition:    condition,
			RawCondition: rawCond,
			Language:     language,
			Stock:        qty,
			ExternalID:   ljID,
		})
	}
	return results
}

// cheapestPerCondition agrupa os listings por condição e retorna o de menor
// preço de cada grupo, na ordem NM → LP → MP → HP → DMG.
func cheapestPerCondition(all []scraper.Result) []scraper.Result {
	best := make(map[string]scraper.Result)
	for _, r := range all {
		if r.Condition == "" {
			continue
		}
		if prev, ok := best[r.Condition]; !ok || r.Price.LessThan(prev.Price) {
			best[r.Condition] = r
		}
	}
	order := []string{
		string(pricing.ConditionNearMint),
		string(pricing.ConditionLightlyPlayed),
		string(pricing.ConditionModeratelyPlayed),
		string(pricing.ConditionHeavilyPlayed),
		string(pricing.ConditionDamaged),
	}
	out := make([]scraper.Result, 0, len(best))
	for _, cond := range order {
		if r, ok := best[cond]; ok {
			out = append(out, r)
		}
	}
	return out
}

// buildTitle monta um título legível para o resultado.
func buildTitle(store, condition, extras, language string) string {
	parts := []string{}
	if store != "" {
		parts = append(parts, store)
	}
	if condition != "" {
		parts = append(parts, condition)
	}
	if extras != "" {
		parts = append(parts, extras)
	}
	if language != "" {
		parts = append(parts, language)
	}
	if len(parts) == 0 {
		return "Liga Pokémon"
	}
	return strings.Join(parts, " · ")
}

// ─── Mapeamentos ──────────────────────────────────────────────────────────────

func mapQualid(q int) string {
	switch q {
	case 1:
		return string(pricing.ConditionNearMint) // Mint → trata como NM
	case 2:
		return string(pricing.ConditionNearMint)
	case 3:
		return string(pricing.ConditionLightlyPlayed)
	case 4:
		return string(pricing.ConditionModeratelyPlayed)
	case 5:
		return string(pricing.ConditionHeavilyPlayed)
	case 6:
		return string(pricing.ConditionDamaged)
	}
	return ""
}

func rawQualid(q int) string {
	switch q {
	case 1:
		return "Mint"
	case 2:
		return "NM"
	case 3:
		return "SP"
	case 4:
		return "MP"
	case 5:
		return "HP"
	case 6:
		return "Damaged"
	}
	return ""
}

func mapIdioma(i int) string {
	switch i {
	case 2:
		return "English"
	case 6:
		return "Japanese"
	case 8:
		return "Portuguese"
	case 11:
		return "PT/EN"
	}
	return ""
}

func mapExtras(e int) string {
	switch e {
	case 2:
		return "Foil"
	case 3:
		return "Reverse Foil"
	case 41:
		return "Shattered Holo"
	case 43:
		return "Master Ball"
	case 47:
		return "Pokeball Foil"
	}
	return ""
}

// ─── Helpers numéricos ────────────────────────────────────────────────────────

// parseBRLNumber converte strings numéricas no formato BR ou EN para Decimal.
// A Liga emite precoFinal como número JSON (ex: 1298.40 ou "1298,40").
func parseBRLNumber(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "null" {
		return decimal.Zero, errors.New("preço zero ou nulo")
	}
	// Formato BR com vírgula decimal
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, err
	}
	return d, nil
}

func toInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

// absoluteURL resolves href (possibly relative, possibly with unencoded chars) to a
// properly-encoded absolute URL. Handles the "./" prefix the Liga HTML uses.
func absoluteURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || href == "#" {
		return ""
	}
	if strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
		return href
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}

	// Split path and query before resolving to handle encoding separately.
	pathPart, queryPart, hasQuery := strings.Cut(href, "?")

	// Strip "./" — Liga HTML uses relative links like "./?view=..."
	pathPart = strings.TrimPrefix(pathPart, "./")

	var absBase string
	if pathPart == "" {
		absBase = base + "/"
	} else if strings.HasPrefix(pathPart, "/") {
		absBase = base + pathPart
	} else {
		absBase = base + "/" + pathPart
	}

	if !hasQuery {
		return absBase
	}

	// Re-encode query parameters: unescape any partial encoding first, then encode cleanly.
	// This turns "card=Pikachu ex (276/217)" into "card=Pikachu+ex+%28276%2F217%29".
	qv := url.Values{}
	for _, pair := range strings.Split(queryPart, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		dk, _ := url.QueryUnescape(k)
		dv, _ := url.QueryUnescape(v)
		qv.Add(dk, dv)
	}
	return absBase + "?" + qv.Encode()
}
