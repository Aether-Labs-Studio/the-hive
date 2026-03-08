package sanitizer

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"the-hive/internal/dht"
)

// ErrIPIPayloadDetected is returned when a chunk is suspected of Indirect Prompt Injection.
var ErrIPIPayloadDetected = errors.New("malicious IPI payload detected: chunk discarded")

// ErrIntegrityViolation is returned when a chunk's hash doesn't match the expected ID.
var ErrIntegrityViolation = errors.New("cryptographic integrity violation: chunk hash mismatch")

// ErrInvalidSignature is returned when a chunk's cryptographic signature is invalid.
var ErrInvalidSignature = errors.New("cryptographic signature violation: invalid authorship")

var (
	// IPI (Indirect Prompt Injection) detection patterns.
	// (?i) for case-insensitivity, (?s) to allow . to match newlines.
	ipiPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?s)ignore\s+(all\s+)?previous\s+instructions`),
		regexp.MustCompile(`(?i)(?s)ignora\s+(todas\s+)?las\s+instrucciones\s+anteriores`),
		regexp.MustCompile(`(?i)(?s)disregard\s+(all\s+)?previous`),
		regexp.MustCompile(`(?i)(?s)you\s+are\s+now\s+a`),
		regexp.MustCompile(`(?i)(?s)ahora\s+eres\s+un`),
		regexp.MustCompile(`(?i)(?s)your\s+(new\s+)?instructions\s+are`),
		regexp.MustCompile(`(?i)(?s)tus\s+(nuevas\s+)?instrucciones\s+son`),
		regexp.MustCompile(`(?i)(?s)system\s+prompt`),
		// Blindaje adicional para Fase 5
		regexp.MustCompile(`(?i)(?s)role\s+hijacking`),
		regexp.MustCompile(`(?i)(?s)new\s+system\s+instructions`),
		regexp.MustCompile(`(?i)(?s)forget\s+(all\s+)?previous\s+context`),
		regexp.MustCompile(`(?i)(?s)tu\s+nuevo\s+rol\s+es`),
	}
)

// ExtractAndInspect decompresses a chunk and scans it for IPI patterns.
func ExtractAndInspect(chunk []byte) ([]byte, error) {
	payload, _, _, err := ExtractAndInspectSecure(chunk, dht.NodeID{})
	return payload, err
}

// ExtractAndInspectSecure decompresses a chunk, verifies its cryptographic integrity
// and authorship signature, and scans it for IPI patterns.
// It returns the raw payload, the verified public key of the author, and the optional parentID.
func ExtractAndInspectSecure(chunk []byte, expectedID dht.NodeID) ([]byte, ed25519.PublicKey, string, error) {
	// 1. Unmarshal Envelope
	var envelope SignedEnvelope
	var dataToProcess []byte
	var author ed25519.PublicKey
	var parentID string

	// Try to unmarshal as signed envelope
	if err := json.Unmarshal(chunk, &envelope); err == nil && len(envelope.Signature) > 0 {
		// Verify Signature
		if !ed25519.Verify(envelope.PublicKey, envelope.Data, envelope.Signature) {
			return nil, nil, "", ErrInvalidSignature
		}
		dataToProcess = envelope.Data
		author = envelope.PublicKey
		parentID = envelope.ParentID
	} else {
		// Fallback for legacy unsigned chunks or direct gzipped data
		dataToProcess = chunk
	}

	// 2. Decompress
	reader, err := gzip.NewReader(bytes.NewReader(dataToProcess))
	if err != nil {
		// Still apply IPI check to raw data
		for _, pattern := range ipiPatterns {
			if pattern.Match(dataToProcess) {
				return nil, nil, "", ErrIPIPayloadDetected
			}
		}
		return dataToProcess, author, parentID, nil 
	}
	defer reader.Close()

	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, "", err
	}

	// 3. Integrity Check (SHA-1)
	var zeroID dht.NodeID
	if expectedID != zeroID {
		actualID := sha1.Sum(payload)
		if actualID != expectedID {
			return nil, nil, "", ErrIntegrityViolation
		}
	}

	// 4. IPI Inspection
	for _, pattern := range ipiPatterns {
		if pattern.Match(payload) {
			return nil, nil, "", ErrIPIPayloadDetected
		}
	}

	return payload, author, parentID, nil
}
