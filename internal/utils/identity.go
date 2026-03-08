package utils

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"the-hive/internal/dht"
)

const (
	pemType = "HIVE IDENTITY PRIVATE KEY"
)

// LoadOrGenerateIdentity attempts to load an Ed25519 private key from the given path.
// If the file does not exist, it generates a new key pair and saves the private key in PEM format.
func LoadOrGenerateIdentity(path string) (ed25519.PrivateKey, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return generateAndSaveIdentity(path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != pemType {
		return nil, errors.New("invalid identity file format")
	}

	if len(block.Bytes) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}

	return ed25519.PrivateKey(block.Bytes), nil
}

// DeriveNodeID generates a DHT NodeID by hashing the public key using SHA-1.
func DeriveNodeID(pub ed25519.PublicKey) dht.NodeID {
	return sha1.Sum(pub)
}

func generateAndSaveIdentity(path string) (ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	block := &pem.Block{
		Type:  pemType,
		Bytes: priv,
	}

	pemData := pem.EncodeToMemory(block)
	// Use strict permissions (0600) for the private key
	if err := os.WriteFile(path, pemData, 0600); err != nil {
		return nil, fmt.Errorf("failed to save identity file: %w", err)
	}

	nodeID := DeriveNodeID(pub)
	fmt.Fprintf(os.Stderr, "  → Nueva identidad generada: %x (Guardada en %s)\n", nodeID, path)

	return priv, nil
}
