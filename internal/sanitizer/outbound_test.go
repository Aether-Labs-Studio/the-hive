package sanitizer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSentinelSanitize(t *testing.T) {
	tempRules := filepath.Join(t.TempDir(), "rules.json")
	s, _ := NewSentinel(tempRules, nil)
	
	input := "My email is test@example.com and my secret is <private>12345</private>. My IP is 192.168.1.1"
	got := string(s.Sanitize([]byte(input)))
	
	if bytes.Contains([]byte(got), []byte("test@example.com")) {
		t.Errorf("Email not redacted")
	}
	if bytes.Contains([]byte(got), []byte("12345")) {
		t.Errorf("Private tag not removed")
	}
	if bytes.Contains([]byte(got), []byte("192.168.1.1")) {
		t.Errorf("IP not redacted")
	}
}

func TestSentinelPackageAndSign(t *testing.T) {
	tempRules := filepath.Join(t.TempDir(), "rules.json")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := NewSentinel(tempRules, priv)

	data := []byte("Relevant shared knowledge.")
	chunk, _, err := s.PackageChunk(data, "parent-id")
	if err != nil {
		t.Fatalf("PackageChunk failed: %v", err)
	}

	// Verify it's a SignedEnvelope
	var env SignedEnvelope
	if err := json.Unmarshal(chunk, &env); err != nil {
		t.Fatalf("Output is not a valid JSON envelope: %v", err)
	}

	if !bytes.Equal(env.PublicKey, pub) {
		t.Errorf("Envelope public key mismatch")
	}

	if !ed25519.Verify(env.PublicKey, env.Data, env.Signature) {
		t.Errorf("Ed25519 signature verification failed")
	}
	
	if env.ParentID != "parent-id" {
		t.Errorf("ParentID not preserved")
	}
}
