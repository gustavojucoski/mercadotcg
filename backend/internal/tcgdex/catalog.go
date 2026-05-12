package tcgdex

import (
	"context"
	"fmt"
)

// BilingualSet combines EN and PT-BR data for a set.
// EN data (Set) is always populated; NamePT and SerieNamePT are empty strings
// when PT-BR is not available for that set (most non-Pocket sets).
type BilingualSet struct {
	Set                   // EN data (authoritative)
	NamePT      string    // set name in PT-BR
	SerieNamePT string    // series name in PT-BR
}

// BilingualCard combines EN (CardRef) and PT-BR data for a card.
// NamePT is empty when PT-BR is not available.
type BilingualCard struct {
	CardRef       // EN data
	NamePT string // card name in PT-BR
}

// EnrichSet fetches the EN set and attempts to augment it with PT-BR translations.
// If PT-BR is not available (404), NamePT and SerieNamePT remain empty — this is
// normal for all non-TCG-Pocket sets.
func EnrichSet(ctx context.Context, c *Client, setID string) (*BilingualSet, error) {
	enSet, err := c.GetSet(ctx, "en", setID)
	if err != nil {
		return nil, fmt.Errorf("enrich set %s (en): %w", setID, err)
	}
	if enSet == nil {
		return nil, nil
	}

	result := &BilingualSet{Set: *enSet}

	ptSet, err := c.GetSet(ctx, "pt-br", setID)
	if err != nil {
		return nil, fmt.Errorf("enrich set %s (pt-br): %w", setID, err)
	}
	if ptSet != nil {
		result.NamePT = ptSet.Name
		if ptSet.Serie.Name != "" {
			result.SerieNamePT = ptSet.Serie.Name
		}
	}

	return result, nil
}

// EnrichCard fetches the PT-BR name for a card given its CardRef (EN data).
// If PT-BR is not available (404), NamePT remains empty.
func EnrichCard(ctx context.Context, c *Client, ref CardRef) (*BilingualCard, error) {
	result := &BilingualCard{CardRef: ref}

	ptCard, err := c.GetCard(ctx, "pt-br", ref.ID)
	if err != nil {
		return nil, fmt.Errorf("enrich card %s (pt-br): %w", ref.ID, err)
	}
	if ptCard != nil {
		result.NamePT = ptCard.Name
	}

	return result, nil
}
