package sanitizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSentinelRulesLoadOrCreate(t *testing.T) {
	tempDir := t.TempDir()
	tempRules := filepath.Join(tempDir, "rules.json")

	// 1. Create default rules
	s, err := NewSentinel(tempRules, nil)
	if err != nil {
		t.Fatalf("Failed to create sentinel: %v", err)
	}

	if len(s.redactRegexes) == 0 {
		t.Errorf("Default redact regexes not loaded")
	}

	if !s.IsTopicBlocked("nomina") {
		t.Errorf("Default blocked topics not loaded")
	}

	// 2. Modify rules and reload
	customRules := Rules{
		RedactPatterns: []string{`secret`},
		BlockedTopics:  []string{"top-secret"},
	}
	data, _ := json.Marshal(customRules)
	os.WriteFile(tempRules, data, 0644)

	s2, err := NewSentinel(tempRules, nil)
	if err != nil {
		t.Fatalf("Failed to reload sentinel: %v", err)
	}

	if len(s2.redactRegexes) != 1 {
		t.Errorf("Custom redact regexes not loaded correctly")
	}

	if !s2.IsTopicBlocked("top-secret") {
		t.Errorf("Custom blocked topics not loaded correctly")
	}
}
