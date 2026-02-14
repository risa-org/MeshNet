package dht

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"
)

type RegisterOptions struct {
	Name string

	Address string

	Services []string

	GroupKey string

	PrivateKey ed25519.PrivateKey
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

	record := Record{
		Name:      opts.Name,
		Address:   opts.Address,
		PublicKey: hex.EncodeToString(pubKey),
		Services:  opts.Services,
		GroupKey:  opts.GroupKey,
		Expires:   time.Now().Add(RecordTTL).Unix(),
	}

	payload := record.SigningPayload()

	signature := ed25519.Sign(opts.PrivateKey, payload)

	record.Signature = hex.EncodeToString(signature)

	return record, nil
}
