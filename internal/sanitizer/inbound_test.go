package sanitizer

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"testing"
	"the-hive/internal/dht"
)

func createSignedChunk(data string, priv ed25519.PrivateKey, parentID string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(data))
	gw.Close()
	compressed := buf.Bytes()

	sig := ed25519.Sign(priv, compressed)
	pub := priv.Public().(ed25519.PublicKey)
	
	envelope := SignedEnvelope{
		Data:      compressed,
		Signature: sig,
		PublicKey: pub,
		ParentID:  parentID,
	}
	res, _ := json.Marshal(envelope)
	return res
}

func TestExtractAndInspectSecure(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	benignText := "Valid content from the swarm."
	validID := dht.NodeID(sha1.Sum([]byte(benignText)))
	parentID := "prev-id"
	signedChunk := createSignedChunk(benignText, priv, parentID)

	t.Run("Valid Signature and Integrity", func(t *testing.T) {
		payload, verifiedPub, gotParent, err := ExtractAndInspectSecure(signedChunk, validID)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if string(payload) != benignText {
			t.Errorf("Payload mismatch: got %q, want %q", string(payload), benignText)
		}
		if !bytes.Equal(verifiedPub, pub) {
			t.Errorf("Verified public key mismatch")
		}
		if gotParent != parentID {
			t.Errorf("ParentID mismatch: got %s, want %s", gotParent, parentID)
		}
	})

	t.Run("Invalid Signature (Tampered Data)", func(t *testing.T) {
		var env SignedEnvelope
		json.Unmarshal(signedChunk, &env)
		// Tamper with gzipped data
		env.Data[0] ^= 0xFF 
		tamperedChunk, _ := json.Marshal(env)

		payload, _, _, err := ExtractAndInspectSecure(tamperedChunk, validID)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("Expected ErrInvalidSignature, got %v", err)
		}
		if payload != nil {
			t.Error("Expected nil payload on signature violation")
		}
	})

	t.Run("Integrity Violation (Mismatched ID)", func(t *testing.T) {
		fakeID := dht.NewNodeID("something else")
		payload, _, _, err := ExtractAndInspectSecure(signedChunk, fakeID)
		if !errors.Is(err, ErrIntegrityViolation) {
			t.Errorf("Expected ErrIntegrityViolation, got %v", err)
		}
		if payload != nil {
			t.Error("Expected nil payload on integrity violation")
		}
	})
}
