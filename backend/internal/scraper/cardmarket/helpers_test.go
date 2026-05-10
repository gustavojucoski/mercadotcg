package cardmarket

import "testing"

func TestSlugContainsNumber(t *testing.T) {
	cases := []struct {
		slug, setCode string
		cardNum       int
		want          bool
	}{
		// exact matches
		{"pikachu-ex-v3-asc276", "ASC", 276, true},
		{"mega-dragonite-ex-v2-asc290", "ASC", 290, true},
		// zero-padded slugs
		{"pikachu-ex-v1-asc057", "ASC", 57, true},
		{"pikachu-ex-v1-asc007", "ASC", 7, true},
		// mismatches
		{"pikachu-ex-v1-asc057", "ASC", 276, false},
		{"mega-dragonite-ex-v1-asc152", "ASC", 290, false},
		{"mega-dragonite-ex-v1-asc152", "ASC", 152, true},
		// set code appears in name (stress test)
		{"ascii-art-v1-asc100", "ASC", 100, true},
		{"ascii-art-v1-asc100", "ASC", 101, false},
	}
	for _, c := range cases {
		got := slugContainsNumber(c.slug, c.setCode, c.cardNum)
		if got != c.want {
			t.Errorf("slugContainsNumber(%q, %q, %d) = %v, want %v",
				c.slug, c.setCode, c.cardNum, got, c.want)
		}
	}
}

func TestExtractVNumber(t *testing.T) {
	cases := []struct {
		slug string
		want int
	}{
		{"pikachu-ex-v1-asc057", 1},
		{"pikachu-ex-v3-asc276", 3},
		{"mega-dragonite-ex-v2-asc290", 2},
		{"charizard-ex-v1-obf125", 1},
		{"no-version-here-asc100", 0},
	}
	for _, c := range cases {
		got := extractVNumber(c.slug)
		if got != c.want {
			t.Errorf("extractVNumber(%q) = %d, want %d", c.slug, got, c.want)
		}
	}
}

func TestParseCardNumber(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"276", 276},
		{"276/217", 276},
		{"057", 57},
		{"7", 7},
		{"290", 290},
	}
	for _, c := range cases {
		got := parseCardNumber(c.s)
		if got != c.want {
			t.Errorf("parseCardNumber(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestPickByVNumber(t *testing.T) {
	// special rare (276 > 217) → highest V
	seen := map[string]struct{}{
		"pikachu-ex-v1-asc057": {},
		"pikachu-ex-v3-asc276": {},
	}
	got := pickByVNumber(seen, "276", 217)
	if got != "pikachu-ex-v3-asc276" {
		t.Errorf("pickByVNumber SIR: got %q, want %q", got, "pikachu-ex-v3-asc276")
	}

	// regular card (57 ≤ 217) → lowest V
	got = pickByVNumber(seen, "57", 217)
	if got != "pikachu-ex-v1-asc057" {
		t.Errorf("pickByVNumber regular: got %q, want %q", got, "pikachu-ex-v1-asc057")
	}

	// unknown printedTotal → ""
	got = pickByVNumber(seen, "276", 0)
	if got != "" {
		t.Errorf("pickByVNumber no printedTotal: got %q, want empty", got)
	}
}
