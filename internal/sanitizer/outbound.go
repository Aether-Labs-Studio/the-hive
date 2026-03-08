package sanitizer

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/json"
	"regexp"
	"time"
	"the-hive/internal/dht"
)

var (
	// Regex for <private>...</private> tags and their content.
	// (?s) allows . to match newlines.
	privateTagRegex = regexp.MustCompile(`(?s)<private>.*?</private>`)

	// Generic email address regex.
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

	// Local IP address regexes (IPv4).
	localIPRegex = regexp.MustCompile(`127\.0\.0\.1|192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2[0-9]|3[0-1])\.\d{1,3}\.\d{1,3}`)
)

const (
	redactedEmail = "[REDACTED_EMAIL]"
	redactedIP    = "[REDACTED_IP]"
)

// SignedEnvelope wraps data with its Ed25519 signature and the author's public key.
type SignedEnvelope struct {
	Data      []byte `json:"data"`       // Gzipped payload or JSON
	Signature []byte `json:"signature"`  // Ed25519 signature
	PublicKey []byte `json:"pub_key"`    // Ed25519 public key
	ExpiresAt int64  `json:"expires_at"` // Unix timestamp for TTL
	ParentID  string `json:"parent_id,omitempty"` // Added for Phase 16 traceability
}

// Sanitize applies static filters (Regex) and custom rules to remove sensitive data.
func (s *Sentinel) Sanitize(raw []byte) []byte {
	// 1. Order matters: extirpate tags first
	res := privateTagRegex.ReplaceAll(raw, []byte(""))

	// 2. Redact emails
	res = emailRegex.ReplaceAll(res, []byte(redactedEmail))

	// 3. Redact local IPs
	res = localIPRegex.ReplaceAll(res, []byte(redactedIP))

	// 4. Apply custom rules
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, re := range s.redactRegexes {
		res = re.ReplaceAll(res, []byte("[REDACTED]"))
	}

	return res
}

// Sign wraps data in a SignedEnvelope without gzipping.
func (s *Sentinel) Sign(data []byte, parentID string) ([]byte, error) {
	if s.privateKey == nil {
		return data, nil
	}

	// Default TTL: 24 hours
	expiresAt := time.Now().Add(24 * time.Hour).Unix()

	sig := ed25519.Sign(s.privateKey, data)
	pub := s.privateKey.Public().(ed25519.PublicKey)
	envelope := SignedEnvelope{
		Data:      data,
		Signature: sig,
		PublicKey: pub,
		ExpiresAt: expiresAt,
		ParentID:  parentID,
	}
	return json.Marshal(envelope)
}

// PackageChunk compresses the sanitized data into gzip format, signs it, and generates its DHT NodeID.
func (s *Sentinel) PackageChunk(sanitized []byte, parentID string) ([]byte, dht.NodeID, error) {
	// 1. Compress
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(sanitized); err != nil {
		return nil, dht.NodeID{}, err
	}
	if err := gw.Close(); err != nil {
		return nil, dht.NodeID{}, err
	}
	compressed := buf.Bytes()

	// 2. Sign
	finalData, err := s.Sign(compressed, parentID)
	if err != nil {
		return nil, dht.NodeID{}, err
	}

	// 3. Generate deterministic NodeID (SHA-1 of the sanitized content)
	id := dht.NewNodeID(string(sanitized))

	return finalData, id, nil
}

// ExtractAndInspect decompresses a chunk and scans it for IPI patterns.
func (s *Sentinel) ExtractAndInspect(chunk []byte) ([]byte, error) {
	return ExtractAndInspect(chunk)
}

// ExtractAndInspectSecure decompresses a chunk, verifies its cryptographic integrity,
// and scans it for IPI patterns.
func (s *Sentinel) ExtractAndInspectSecure(chunk []byte, expectedID dht.NodeID) ([]byte, ed25519.PublicKey, string, error) {
	return ExtractAndInspectSecure(chunk, expectedID)
}

// Sanitize applies static filters (Regex) to remove sensitive data.
func Sanitize(raw []byte) []byte {
	res := privateTagRegex.ReplaceAll(raw, []byte(""))
	res = emailRegex.ReplaceAll(res, []byte(redactedEmail))
	res = localIPRegex.ReplaceAll(res, []byte(redactedIP))
	return res
}
