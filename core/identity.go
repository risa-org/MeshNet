package core

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Identity struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

const identityFile = "identity.json"

func loadOrCreateIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	data, err := os.ReadFile(identityFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return createAndSaveIdentity()
		}
		return nil, nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, nil, fmt.Errorf("failed to parse identity file: %w", err)
	}

	privKeyBytes, err := hex.DecodeString(identity.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	pubKeyBytes, err := hex.DecodeString(identity.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := ed25519.PublicKey(pubKeyBytes)

	fmt.Println("Identity loaded from disk")
	return pubKey, privKey, nil
}

func createAndSaveIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate keys: %w", err)
	}

	identity := Identity{
		PrivateKey: hex.EncodeToString(privKey),
		PublicKey:  hex.EncodeToString(pubKey),
	}

	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode identity: %w", err)
	}

	if err := os.WriteFile(identityFile, data, 0600); err != nil {
		return nil, nil, fmt.Errorf("failed to save identity: %w", err)
	}

	fmt.Println("Fresh identity generated and saved to", identityFile)
	return pubKey, privKey, nil
}
