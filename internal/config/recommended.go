package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed recommended_rules.json
var recommendedRulesJSON []byte

// RecommendedRules returns the built-in set of recommended routing rules.
func RecommendedRules() (RuleList, error) {
	var rules RuleList
	if err := json.Unmarshal(recommendedRulesJSON, &rules); err != nil {
		return nil, fmt.Errorf("parse recommended rules: %w", err)
	}
	return rules, nil
}
