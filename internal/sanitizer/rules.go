package sanitizer

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"the-hive/internal/logger"
)

// Rules defines the structure of the rules.json file.
type Rules struct {
	RedactPatterns []string `json:"redact_patterns"`
	BlockedTopics  []string `json:"blocked_topics"`
}

// Sentinel handles data sanitization, policy enforcement, and cryptographic signing.
type Sentinel struct {
	rules          Rules
	redactRegexes  []*regexp.Regexp
	blockedTopics  map[string]bool
	privateKey     ed25519.PrivateKey
	mu             sync.RWMutex
}

// NewSentinel creates a new Sentinel instance by loading rules from the specified path.
func NewSentinel(rulesPath string, priv ed25519.PrivateKey) (*Sentinel, error) {
	s := &Sentinel{
		blockedTopics: make(map[string]bool),
		privateKey:    priv,
	}

	if err := s.loadOrCreateRules(rulesPath); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Sentinel) loadOrCreateRules(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create default rules
		defaultRules := Rules{
			RedactPatterns: []string{
				`(?i)confidencial`,
				`(?i)password\s*=\s*\S+`,
			},
			BlockedTopics: []string{
				"nomina",
				"estrategia-militar",
			},
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		data, err := json.MarshalIndent(defaultRules, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
		logger.Info("Sentinel: Creado archivo de reglas por defecto en %s", path)
		s.rules = defaultRules
	} else {
		// Load existing rules
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, &s.rules); err != nil {
			return err
		}
		logger.Info("Sentinel: Reglas cargadas desde %s", path)
	}

	// Pre-compile regexes
	s.redactRegexes = make([]*regexp.Regexp, 0, len(s.rules.RedactPatterns))
	for _, pattern := range s.rules.RedactPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Error("Sentinel: Error al compilar regex '%s': %v", pattern, err)
			continue
		}
		s.redactRegexes = append(s.redactRegexes, re)
	}

	// Map blocked topics for O(1) lookup
	s.blockedTopics = make(map[string]bool)
	for _, topic := range s.rules.BlockedTopics {
		s.blockedTopics[topic] = true
	}

	return nil
}

// IsTopicBlocked returns true if the given topic is in the blocked list.
func (s *Sentinel) IsTopicBlocked(topic string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockedTopics[topic]
}
