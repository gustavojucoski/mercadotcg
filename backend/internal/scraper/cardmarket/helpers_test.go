package cardmarket

import (
	"fmt"
	"testing"
)

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

func TestInjectVersion(t *testing.T) {
	const base = "https://www.cardmarket.com/en/Pokemon/Products/Singles"
	cases := []struct {
		target, setCode string
		n               int
		want            string
	}{
		// Set code present in slug → insert before -SETCODE
		{base + "/Ascended-Heroes/Pikachu-ex-ASC276", "ASC", 1, base + "/Ascended-Heroes/Pikachu-ex-V1-ASC276"},
		{base + "/Ascended-Heroes/Pikachu-ex-ASC276", "ASC", 3, base + "/Ascended-Heroes/Pikachu-ex-V3-ASC276"},
		{base + "/Ascended-Heroes/Pikachu-ex-ASC057", "ASC", 2, base + "/Ascended-Heroes/Pikachu-ex-V2-ASC057"},
		// n=10
		{base + "/Ascended-Heroes/Pikachu-ex-ASC276", "ASC", 10, base + "/Ascended-Heroes/Pikachu-ex-V10-ASC276"},
		// Set code absent from slug → append at end
		{base + "/Sword-Shield/Charizard-VMAX", "SSH", 1, base + "/Sword-Shield/Charizard-VMAX-V1"},
		// Empty set code → append
		{base + "/Sword-Shield/Charizard-VMAX", "", 2, base + "/Sword-Shield/Charizard-VMAX-V2"},
	}
	for _, c := range cases {
		got := injectVersion(c.target, c.setCode, c.n)
		if got != c.want {
			t.Errorf("injectVersion(target, %q, %d)\n  got  %q\n  want %q",
				c.setCode, c.n, got, c.want)
		}
	}
}

// Verify that injectVersion produces distinct URLs for V1..V10 (no collisions).
func TestInjectVersionDistinct(t *testing.T) {
	target := "https://www.cardmarket.com/en/Pokemon/Products/Singles/Ascended-Heroes/Pikachu-ex-ASC276"
	seen := map[string]int{}
	for n := 1; n <= 10; n++ {
		u := injectVersion(target, "ASC", n)
		if prev, ok := seen[u]; ok {
			t.Errorf("injectVersion collision: n=%d and n=%d produce same URL %q", prev, n, u)
		}
		seen[u] = n
		want := fmt.Sprintf("https://www.cardmarket.com/en/Pokemon/Products/Singles/Ascended-Heroes/Pikachu-ex-V%d-ASC276", n)
		if u != want {
			t.Errorf("n=%d: got %q, want %q", n, u, want)
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
