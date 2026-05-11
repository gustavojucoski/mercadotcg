package main

import (
	"encoding/json"
	"os"
)

// RuleMap mapeia: tcg → series (ou "_default") → rarity → []finish string.
// Permite definir quais variantes de acabamento existem para cada rarity de um TCG/série.
type RuleMap map[string]map[string]map[string][]string

// parseRules lê e decodifica o arquivo JSON de regras de variante.
func parseRules(path string) (RuleMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules RuleMap
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// resolveFinishes retorna os finishes para uma carta dado tcg, series e rarity.
//
// Ordem de lookup:
//  1. tcg → series → rarity
//  2. tcg → "_default" → rarity
//  3. fallback: ["normal"]
func resolveFinishes(tcg, series, rarity string, rules RuleMap) []string {
	tcgRules, ok := rules[tcg]
	if !ok {
		return []string{"normal"}
	}
	if seriesRules, ok := tcgRules[series]; ok {
		if finishes, ok := seriesRules[rarity]; ok {
			return finishes
		}
	}
	if defaultRules, ok := tcgRules["_default"]; ok {
		if finishes, ok := defaultRules[rarity]; ok {
			return finishes
		}
	}
	return []string{"normal"}
}
