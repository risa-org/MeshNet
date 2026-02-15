package core

import (
	"bytes"
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

func identityFilePath() string {
	path := os.Getenv("IDENTITY")
	if path == "" {
		return "identity.json"
	}
	return path
}

func loadOrCreateIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	// try installed Yggdrasil's identity first
	// if found, we share one address with the OS-level mesh interface
	pubKey, privKey, err := tryReadYggdrasilIdentity()
	if err != nil {
		return nil, nil, err
	}
	if pubKey != nil {
		return pubKey, privKey, nil
	}

	// fall back to our own identity file
	data, err := os.ReadFile(identityFilePath())
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

	privKey = ed25519.PrivateKey(privKeyBytes)
	pubKey = ed25519.PublicKey(pubKeyBytes)

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

	if err := os.WriteFile(identityFilePath(), data, 0600); err != nil {
		return nil, nil, fmt.Errorf("failed to save identity: %w", err)
	}

	fmt.Println("Fresh identity generated and saved to", identityFilePath())
	return pubKey, privKey, nil
}

// PrivKeyHex returns the private key as a hex string
// used when writing Yggdrasil config file
func PrivKeyHex(privKey ed25519.PrivateKey) string {
	return hex.EncodeToString(privKey)
}

// yggdrasilConfigPath is where the installed Yggdrasil stores its config
const yggdrasilConfigPath = `C:\ProgramData\Yggdrasil\yggdrasil.conf`

// tryReadYggdrasilIdentity attempts to read the keypair from an installed
// Yggdrasil instance. Returns nil, nil, nil if not found — caller falls back.
func tryReadYggdrasilIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	data, err := os.ReadFile(yggdrasilConfigPath)
	if err != nil {
		// not installed or not readable — fall back to our own identity
		return nil, nil, nil
	}

	// Yggdrasil config is NOT standard JSON — it uses a custom format
	// with unquoted keys and # comments
	// we just scan line by line for the PrivateKey field
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)

		// skip comments and empty lines
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}

		// look for:  PrivateKey: <hexstring>
		if !bytes.HasPrefix(trimmed, []byte("PrivateKey:")) {
			continue
		}

		// extract the value after the colon
		parts := bytes.SplitN(trimmed, []byte(":"), 2)
		if len(parts) != 2 {
			continue
		}

		keyHex := string(bytes.TrimSpace(parts[1]))
		if keyHex == "" {
			continue
		}

		privKeyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid private key in yggdrasil config: %w", err)
		}

		privKey := ed25519.PrivateKey(privKeyBytes)
		pubKey := privKey.Public().(ed25519.PublicKey)

		fmt.Println("Identity loaded from installed Yggdrasil")
		return pubKey, privKey, nil
	}

	// PrivateKey line not found
	return nil, nil, fmt.Errorf("PrivateKey not found in yggdrasil config")
}
