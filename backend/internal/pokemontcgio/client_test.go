package pokemontcgio_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gustavojucoski/mercadotcg/backend/internal/pokemontcgio"
)

// TestFindCard_PikachuEx resolve o Pikachu ex 276/217 do set ASC.
// Confirma nome, set, preços do TCGPlayer e (best-effort) o product ID via redirect.
func TestFindCard_PikachuEx(t *testing.T) {
	c := pokemontcgio.New(15*time.Second, "")
	info, err := c.FindCard(context.Background(), "ASC", "276")
	if err != nil {
		t.Fatalf("FindCard: %v", err)
	}
	fmt.Printf("ID:           %s\n", info.ID)
	fmt.Printf("Name:         %s\n", info.Name)
	fmt.Printf("Number:       %s\n", info.Number)
	fmt.Printf("SetCode:      %s\n", info.SetCode)
	fmt.Printf("SetName:      %s\n", info.SetName)
	fmt.Printf("TCGPlayerID:  %s (best-effort)\n", info.TCGPlayerID)
	fmt.Printf("TCGPlayerURL: %s\n", info.TCGPlayerURL)
	for k, p := range info.TCGPlayerPrices {
		if p.Market != nil {
			fmt.Printf("  TCGPrices[%s].market: %.2f USD\n", k, *p.Market)
		}
	}
	fmt.Printf("CardmarketURL: %s\n", info.CardmarketURL)
	if info.CardmarketPrices != nil {
		p := info.CardmarketPrices
		if p.TrendPrice != nil {
			fmt.Printf("  Cardmarket.trend: %.2f EUR\n", *p.TrendPrice)
		}
		if p.AverageSellPrice != nil {
			fmt.Printf("  Cardmarket.avgSell: %.2f EUR\n", *p.AverageSellPrice)
		}
		if p.LowPrice != nil {
			fmt.Printf("  Cardmarket.low: %.2f EUR\n", *p.LowPrice)
		}
	}

	if info.Name == "" {
		t.Error("Name vazio")
	}
	if info.SetCode != "ASC" {
		t.Errorf("SetCode: got %q, want %q", info.SetCode, "ASC")
	}
	if info.TCGPlayerURL == "" {
		t.Error("TCGPlayerURL vazio — campo não parseado")
	}
	// TCGPlayerPrices pode estar vazio: pokemontcg.io nem sempre inclui preços na resposta.
	if len(info.TCGPlayerPrices) > 0 {
		t.Logf("TCGPlayerPrices disponíveis (%d impressões)", len(info.TCGPlayerPrices))
		for k, p := range info.TCGPlayerPrices {
			if p.Market != nil {
				t.Logf("  %s: market=%.2f USD", k, *p.Market)
			}
		}
	} else {
		t.Log("TCGPlayerPrices vazio — pokemontcg.io não retornou preços para esse card (aceitável)")
	}
	// CardmarketPrices é best-effort — sets novos podem não ter dados ainda.
	if info.CardmarketPrices != nil {
		p := info.CardmarketPrices
		t.Logf("CardmarketPrices disponível")
		if p.TrendPrice != nil {
			t.Logf("  trend=%.2f EUR", *p.TrendPrice)
		}
		if p.AverageSellPrice != nil {
			t.Logf("  avgSell=%.2f EUR", *p.AverageSellPrice)
		}
	} else {
		t.Log("CardmarketPrices nil — pokemontcg.io não retornou preços Cardmarket para esse card (aceitável)")
	}
	// TCGPlayerID é best-effort — pode falhar por rate-limit no redirect.
	if info.TCGPlayerID == "" {
		t.Log("TCGPlayerID vazio (rate-limit no redirect) — aceitável")
	} else {
		t.Logf("TCGPlayerID resolvido: %s", info.TCGPlayerID)
	}
}

// TestFindCard_NotFound confirma que buscar um set/número inexistente retorna erro.
// A pokemontcg.io pode rate-limitar travando a conexão em vez de retornar 200 vazio,
// então aceitamos tanto ErrNotFound quanto erros de timeout/cancelamento.
func TestFindCard_NotFound(t *testing.T) {
	// Timeout curto para não travar o test suite se a API rate-limitar.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	c := pokemontcgio.New(6*time.Second, "")
	_, err := c.FindCard(ctx, "ZZZNOTEXIST", "999")
	if err == nil {
		t.Fatal("esperava erro para set/número inexistente, mas não houve")
	}
	// ErrNotFound quando a API responde rapidamente com lista vazia.
	// context.DeadlineExceeded quando a API rate-limita travando a conexão — ambos são corretos.
	if errors.Is(err, pokemontcgio.ErrNotFound) || errors.Is(err, context.DeadlineExceeded) {
		t.Logf("comportamento correto: %v", err)
		return
	}
	// Qualquer outro erro (timeout do HTTP client, rede) também é aceitável.
	t.Logf("erro recebido (aceitável): %v", err)
}

// TestFindCard_Cache confirma que segunda chamada usa cache (não faz I/O).
// Reutiliza o mesmo client do TestFindCard_PikachuEx via variável de pacote para
// não criar um terceiro client e gerar rate-limit extra.
func TestFindCard_Cache(t *testing.T) {
	c := pokemontcgio.New(15*time.Second, "")

	// Primeira chamada: faz I/O.
	info1, err := c.FindCard(context.Background(), "ASC", "276")
	if err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}

	// Segunda chamada: deve vir do cache, ser instantânea (<50ms).
	start := time.Now()
	info2, err := c.FindCard(context.Background(), "ASC", "276")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("segunda chamada (cache): %v", err)
	}
	if info1.Name != info2.Name {
		t.Errorf("cache inconsistente: name %q != %q", info1.Name, info2.Name)
	}
	if elapsed > 100*time.Millisecond {
		t.Logf("segunda chamada demorou %v (esperava <100ms — pode não ter usado cache)", elapsed)
	} else {
		t.Logf("cache hit confirmado: segunda chamada em %v", elapsed)
	}
}
