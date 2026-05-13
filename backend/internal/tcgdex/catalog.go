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

	// PT-BR is optional enrichment; 404 is expected for non-Pocket sets.
	// Any transient error (5xx, timeout) is silently ignored so the EN set
	// is still persisted rather than skipped entirely.
	ptSet, _ := c.GetSet(ctx, "pt-br", setID)
	if ptSet != nil {
		result.NamePT = ptSet.Name
		if ptSet.Serie.Name != "" {
			result.SerieNamePT = ptSet.Serie.Name
		}
	}

	return result, nil
}

