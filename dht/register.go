package dht

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"
)

// RegisterOptions defines the parameters for creating a DHT record
type RegisterOptions struct {
	Name       string
	Address    string
	Services   []string
	GroupKey   string
	PrivateKey ed25519.PrivateKey
	TTL        time.Duration // optional — 0 means use default RecordTTL
}

func CreateRecord(opts RegisterOptions) (Record, error) {
	if opts.Name == "" {
		return Record{}, fmt.Errorf("name cannot be empty")
	}
	if opts.Address == "" {
		return Record{}, fmt.Errorf("address cannot be empty")
	}
	if opts.PrivateKey == nil {
		return Record{}, fmt.Errorf("private key cannot be nil")
	}

	pubKey := opts.PrivateKey.Public().(ed25519.PublicKey)

	// use custom TTL if provided, otherwise default
	ttl := opts.TTL
	if ttl == 0 {
		ttl = RecordTTL
	}

	record := Record{
		Name:      opts.Name,
		Address:   opts.Address,
		PublicKey: hex.EncodeToString(pubKey),
		Services:  opts.Services,
		GroupKey:  opts.GroupKey,
		Expires:   time.Now().Add(ttl).Unix(),
	}

	payload := record.SigningPayload()

	signature := ed25519.Sign(opts.PrivateKey, payload)

	record.Signature = hex.EncodeToString(signature)

	return record, nil
}
