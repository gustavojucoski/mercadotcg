package forex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
)

// BCBProvider consulta o serviço PTAX do Banco Central do Brasil.
//
// Endpoint usado:
//
//	GET https://olinda.bcb.gov.br/olinda/servico/PTAX/versao/v1/odata/
//	    CotacaoMoedaDia(moeda=@moeda,dataCotacao=@dataCotacao)
//	      ?@moeda='USD'&@dataCotacao='05-09-2026'&$format=json
//
// O BCB não publica cotação em fins de semana/feriados — nesses casos a
// resposta vem com `value` vazio e devolvemos ErrRateUnavailable; o Service
// trata o fallback para o dia útil anterior.
type BCBProvider struct {
	client  *http.Client
	baseURL string
}

// NewBCBProvider monta o cliente HTTP. O timeout é por requisição.
func NewBCBProvider(timeout time.Duration) *BCBProvider {
	return &BCBProvider{
		client: &http.Client{Timeout: timeout},
		baseURL: "https://olinda.bcb.gov.br/olinda/servico/PTAX/versao/v1/odata/" +
			"CotacaoMoedaDia(moeda=@moeda,dataCotacao=@dataCotacao)",
	}
}

// Name devolve o identificador usado em forex_rates.source.
func (p *BCBProvider) Name() string { return "bcb" }

// bcbResponse mapeia a resposta OData do PTAX. Apenas os campos que usamos.
type bcbResponse struct {
	Value []struct {
		CotacaoCompra    float64 `json:"cotacaoCompra"`
		CotacaoVenda     float64 `json:"cotacaoVenda"`
		DataHoraCotacao  string  `json:"dataHoraCotacao"`
		TipoBoletim      string  `json:"tipoBoletim"`
	} `json:"value"`
}

// Fetch implementa Provider. Usa cotacaoVenda como referência (taxa pela qual
// o BCB venderia 1 unidade da moeda em BRL — convenção mais conservadora).
func (p *BCBProvider) Fetch(ctx context.Context, currency string, day time.Time) (Quote, error) {
	// PTAX espera MM-DD-YYYY entre aspas simples na URL.
	dateParam := fmt.Sprintf("'%s'", day.Format("01-02-2006"))
	moedaParam := fmt.Sprintf("'%s'", currency)

	q := url.Values{}
	q.Set("@moeda", moedaParam)
	q.Set("@dataCotacao", dateParam)
	q.Set("$format", "json")
	q.Set("$select", "cotacaoCompra,cotacaoVenda,dataHoraCotacao,tipoBoletim")

	reqURL := p.baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("bcb: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("bcb: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("bcb: status %d", resp.StatusCode)
	}

	var body bcbResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Quote{}, fmt.Errorf("bcb: decode: %w", err)
	}

	if len(body.Value) == 0 {
		// Sem cotação para o dia (fim de semana/feriado).
		return Quote{}, ErrRateUnavailable
	}

	// Em dias com vários boletins (intermediário e fechamento), o BCB devolve
	// múltiplos itens. Preferimos o "Fechamento PTAX"; cai para o último item.
	rec := body.Value[len(body.Value)-1]
	for _, c := range body.Value {
		if c.TipoBoletim == "Fechamento PTAX" {
			rec = c
			break
		}
	}

	if rec.CotacaoVenda <= 0 {
		return Quote{}, errors.New("bcb: cotacaoVenda inválida")
	}

	rate := decimal.NewFromFloat(rec.CotacaoVenda)

	return Quote{
		Currency:  currency,
		RateToBRL: rate,
		QuotedAt:  day,
		Source:    p.Name(),
	}, nil
}
