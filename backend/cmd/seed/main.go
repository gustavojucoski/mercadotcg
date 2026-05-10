// Command seed popula o banco com um conjunto realista de dados de demo:
// 1 set, 3 cartas, ~6 variantes, sua loja, ~30 dias de price_daily nas
// 3 fontes (LigaPokemon, TCGplayer, eBay) — para o /signal devolver algo.
//
// Idempotente: pode rodar mais de uma vez. Usa UUIDs determinísticos
// derivados de strings para que reexecuções não dupliquem.
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/config"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/card"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/matching"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

var ns = uuid.NewSHA1(uuid.NameSpaceURL, []byte("mercadotcg.seed"))

func deterministicUUID(label string) uuid.UUID {
	return uuid.NewSHA1(ns, []byte(label))
}

func main() {
	cfg, err := config.Load()
	must(err, "config")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	must(err, "connect postgres")
	defer pool.Close()

	cardRepo := postgres.NewCardRepo(pool)
	storeRepo := postgres.NewStoreRepo(pool)
	stockRepo := postgres.NewStockRepo(pool)
	dailyRepo := postgres.NewPriceDailyRepo(pool)
	refRepo := postgres.NewExternalRefRepo(pool)
	forexRepo := postgres.NewForexRepo(pool)

	fmt.Println("==> Seedando MercadoTCG demo data")

	// ---- Forex de hoje (1 USD = 5,40 BRL fictício) ------------------------
	if err := forexRepo.Upsert(ctx, &postgres.ForexRate{
		Currency:  "USD",
		RateToBRL: decimal.RequireFromString("5.4000"),
		QuotedAt:  time.Now().UTC().Truncate(24 * time.Hour),
		Source:    "seed",
	}); err != nil {
		exit("forex usd: %v", err)
	}

	// ---- Set --------------------------------------------------------------
	setCode := "sv8"
	setObj, err := cardRepo.GetSetByCode(ctx, setCode)
	if errors.Is(err, postgres.ErrNotFound) {
		setObj = card.Set{
			Code:     setCode,
			Name:     "Surging Sparks",
			Series:   "Scarlet & Violet",
			Language: card.LanguageEnglish,
		}
		now := time.Date(2024, 11, 8, 0, 0, 0, 0, time.UTC)
		setObj.ReleaseDate = &now
		setObj.TotalCards = 252
		if err := cardRepo.CreateSet(ctx, &setObj); err != nil {
			exit("create set: %v", err)
		}
		fmt.Printf("    set %s criado: %s\n", setObj.Code, setObj.ID)
	} else if err != nil {
		exit("get set: %v", err)
	} else {
		fmt.Printf("    set %s já existe: %s\n", setObj.Code, setObj.ID)
	}

	// ---- Cartas + variantes ----------------------------------------------
	type cardSpec struct {
		number   string
		name     string
		rarity   string
		variants []card.Finish // pelo menos 1
	}
	specs := []cardSpec{
		{"025/191", "Pikachu ex", "Special Illustration Rare", []card.Finish{card.FinishHolo, card.FinishMasterBallMirror, card.FinishPokeBallMirror}},
		{"199/191", "Charizard ex", "Hyper Rare", []card.Finish{card.FinishHolo, card.FinishGoldEtched}},
		{"110/191", "Lucario", "Common", []card.Finish{card.FinishNormal, card.FinishReverseHolo}},
	}

	type variantHandle struct {
		card    card.Card
		finish  card.Finish
		variant card.Variant
	}
	var variants []variantHandle

	for _, sp := range specs {
		c := card.Card{
			SetID:  setObj.ID,
			Number: sp.number,
			Name:   sp.name,
			Rarity: sp.rarity,
		}
		err := cardRepo.CreateCard(ctx, &c)
		if errors.Is(err, postgres.ErrAlreadyExists) {
			// já existe — a query GetCardByID exige UUID, então buscamos via search.
			results, serr := cardRepo.SearchCardsByName(ctx, sp.name, 5)
			if serr != nil {
				exit("buscar carta %s: %v", sp.name, serr)
			}
			for _, r := range results {
				if r.SetID == setObj.ID && r.Number == sp.number {
					c = r
					break
				}
			}
			fmt.Printf("    carta %s já existe: %s\n", sp.name, c.ID)
		} else if err != nil {
			exit("create card %s: %v", sp.name, err)
		} else {
			fmt.Printf("    carta %s criada: %s\n", sp.name, c.ID)
		}

		for _, finish := range sp.variants {
			v := card.Variant{CardID: c.ID, Finish: finish}
			err := cardRepo.CreateVariant(ctx, &v)
			if errors.Is(err, postgres.ErrAlreadyExists) {
				existing, lerr := cardRepo.ListVariantsByCard(ctx, c.ID)
				if lerr != nil {
					exit("list variants %s: %v", c.Name, lerr)
				}
				for _, ev := range existing {
					if ev.Finish == finish {
						v = ev
						break
					}
				}
			} else if err != nil {
				exit("create variant %s/%s: %v", c.Name, finish, err)
			}
			variants = append(variants, variantHandle{card: c, finish: finish, variant: v})
		}
	}
	fmt.Printf("    %d variantes prontas\n", len(variants))

	// ---- Loja do usuário --------------------------------------------------
	// freshSeed = true significa que esta é a primeira execução do seed
	// (a loja não existia). Idempotência: nas execuções seguintes, pulamos
	// o loop de RegisterPurchase para não inflar o estoque.
	ownerID := deterministicUUID("owner:gustavo")
	mySlug := "mercado-do-gus"
	myStore, err := storeRepo.GetBySlug(ctx, mySlug)
	freshSeed := false
	if errors.Is(err, postgres.ErrNotFound) {
		freshSeed = true
		myStore = store.Store{
			OwnerID:     ownerID,
			Name:        "Mercado do Gus",
			Slug:        mySlug,
			Description: "Loja de demonstração — gerada via cmd/seed.",
			IsActive:    true,
		}
		if err := storeRepo.Create(ctx, &myStore); err != nil {
			exit("create store: %v", err)
		}
		fmt.Printf("    loja '%s' criada: %s (owner=%s)\n", myStore.Name, myStore.ID, ownerID)
	} else if err != nil {
		exit("get store: %v", err)
	} else {
		fmt.Printf("    loja '%s' já existe: %s — pulando insert de estoque\n", myStore.Name, myStore.ID)
	}

	// ---- Estoque inicial — uma compra de cada variante --------------------
	// Só roda na primeira execução. Re-rodar somaria quantidade infinitamente.
	if freshSeed {
		for _, vh := range variants {
			cost := decimal.NewFromInt(int64(rand.Intn(120) + 30)) // R$ 30-150
			if _, err := stockRepo.RegisterPurchase(ctx, postgres.PurchaseInput{
				StoreID:      myStore.ID,
				VariantID:    vh.variant.ID,
				Condition:    string(pricing.ConditionNearMint),
				Language:     "en",
				Quantity:     2,
				UnitCostBRL:  cost,
				ReferenceTyp: "seed",
				Notes:        fmt.Sprintf("Seed: %s (%s)", vh.card.Name, vh.finish),
			}); err != nil {
				fmt.Printf("    estoque %s/%s: %v\n", vh.card.Name, vh.finish, err)
			}
		}
		fmt.Printf("    estoque inicial criado (%d itens)\n", len(variants))
	}

	// ---- External refs (matching simulado) --------------------------------
	srcOf := []pricing.Source{pricing.SourceLigaPokemon, pricing.SourceTCGPlayer, pricing.SourceEbay}
	for _, vh := range variants {
		for _, src := range srcOf {
			ref := &matching.ExternalCardRef{
				VariantID:  vh.variant.ID,
				Source:     src,
				ExternalID: fmt.Sprintf("%s-%s-%s", src, vh.card.Number, vh.finish),
				Language:   "en",
				Confidence: 100,
				RawTitle:   fmt.Sprintf("%s (%s)", vh.card.Name, vh.finish),
			}
			if err := refRepo.Create(ctx, ref); err != nil && !errors.Is(err, postgres.ErrAlreadyExists) {
				exit("create ref: %v", err)
			}
		}
	}
	fmt.Printf("    %d external refs criadas (3 fontes × %d variantes)\n", 3*len(variants), len(variants))

	// ---- price_daily simulado (30 dias × 3 fontes × 3 condições × variantes) ----
	rand.Seed(42) // determinístico para reseeds compararem
	today := time.Now().UTC().Truncate(24 * time.Hour)

	// fonte → fator multiplicativo (TCG fica mais cara, eBay no meio)
	sourceBias := map[pricing.Source]float64{
		pricing.SourceLigaPokemon: 1.00,
		pricing.SourceTCGPlayer:   1.12,
		pricing.SourceEbay:        1.05,
	}
	// condição → fator (LP ~85% NM, MP ~70% NM)
	conditionBias := []struct {
		cond  pricing.Condition
		factor float64
	}{
		{pricing.ConditionNearMint, 1.00},
		{pricing.ConditionLightlyPlayed, 0.85},
		{pricing.ConditionModeratelyPlayed, 0.70},
	}

	rows := 0
	for _, vh := range variants {
		basePrice := decimal.NewFromInt(int64(rand.Intn(400) + 100)) // R$ 100-500
		for _, src := range srcOf {
			for _, cb := range conditionBias {
				bias := sourceBias[src] * cb.factor
				for d := 0; d < 30; d++ {
					day := today.AddDate(0, 0, -d)
					jitter := 0.90 + rand.Float64()*0.20 // ±10%
					avg := basePrice.Mul(decimal.NewFromFloat(bias * jitter)).Round(2)

					saleMin := avg.Mul(decimal.NewFromFloat(0.90)).Round(2)
					saleMax := avg.Mul(decimal.NewFromFloat(1.10)).Round(2)
					// MP tem menos vendas que LP que tem menos que NM
					maxSales := map[pricing.Condition]int{
						pricing.ConditionNearMint:         8,
						pricing.ConditionLightlyPlayed:    5,
						pricing.ConditionModeratelyPlayed: 3,
					}[cb.cond]
					salesCount := rand.Intn(maxSales) + 1

					p := pricing.DailyPoint{
						VariantID:     vh.variant.ID,
						Condition:     cb.cond,
						Source:        src,
						Day:           day,
						SalesCount:    salesCount,
						ListingsCount: salesCount * 2,
						SaleMin:       &saleMin,
						SaleMax:       &saleMax,
						SaleAvg:       &avg,
						SaleMedian:    &avg,
						SaleP25:       &saleMin,
						SaleP75:       &saleMax,
					}
					if err := dailyRepo.Upsert(ctx, p); err != nil {
						exit("upsert daily: %v", err)
					}
					rows++
				}
			}
		}
	}
	fmt.Printf("    %d linhas em price_daily geradas (3 condições × 3 fontes × 30 dias × %d variantes)\n",
		rows, len(variants))

	fmt.Println()
	fmt.Println("==> Seed completo!")
	fmt.Println()
	fmt.Println("    Sua loja:    ", myStore.ID)
	fmt.Println("    Owner UUID:  ", ownerID)
	fmt.Println("    Set:         ", setObj.ID)
	fmt.Println()
	fmt.Println("Endpoints úteis:")
	fmt.Printf("    GET  /healthz\n")
	fmt.Printf("    GET  /api/v1/cards/search?q=charizard\n")
	fmt.Printf("    GET  /api/v1/cards/lookup?name=charizard&with_signal=true\n")
	fmt.Printf("    GET  /api/v1/cards/lookup?set=sv8&number=199/191&with_signal=true\n")
	fmt.Printf("    GET  /api/v1/stores/%s/stock?with_signal=true\n", myStore.ID)
	if len(variants) > 0 {
		fmt.Printf("    GET  /api/v1/variants/%s/signal\n", variants[0].variant.ID)
	}
}

func must(err error, msg string) {
	if err != nil {
		exit("%s: %v", msg, err)
	}
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "seed: "+format+"\n", args...)
	os.Exit(1)
}

