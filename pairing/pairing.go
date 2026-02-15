package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"meshnet/dht"
)

const (
	// PairingTTL is how long pairing records live in the DHT
	// long enough for both parties to exchange but short enough
	// not to pollute the DHT permanently
	PairingTTL = 10 * time.Minute

	// PairingTimeout is how long we wait for the other party
	PairingTimeout = 5 * time.Minute

	// PollInterval is how often we check for the response
	PollInterval = 2 * time.Second
)

// PairingRecord is what gets stored in the DHT during pairing
// contains enough info for the other party to add us as a contact
type PairingRecord struct {
	Name       string `json:"name"`
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	Code       string `json:"code"`
	IsResponse bool   `json:"is_response"`
}

// GenerateCode generates a human-readable pairing code
// format: MESH-XXXX where XXXX is 4 random uppercase alphanumeric chars
// easy to type, hard to guess, low collision probability for short sessions
func GenerateCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// deliberately excludes I, O, 0, 1 — easy to confuse visually
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate code: %w", err)
	}
	result := make([]byte, 4)
	for i, v := range b {
		result[i] = charset[int(v)%len(charset)]
	}
	return "MESH-" + string(result), nil
}

// Initiate starts a pairing session as the initiator
// generates a code, announces it to DHT, waits for response
// returns the paired contact on success
func Initiate(
	d *dht.DHT,
	name string,
	address string,
	privKey ed25519.PrivateKey,
) (*Contact, error) {
	// generate pairing code
	code, err := GenerateCode()
	if err != nil {
		return nil, err
	}

	fmt.Printf("\nYour pairing code: %s\n", code)
	fmt.Println("Share this code with the other device.")
	fmt.Printf("Waiting for partner... (expires in %s)\n\n", PairingTimeout)

	// create and announce our pairing record
	record, err := createPairingRecord(name, address, privKey, code, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create pairing record: %w", err)
	}

	if err := d.Announce(record); err != nil {
		return nil, fmt.Errorf("failed to announce pairing record: %w", err)
	}

	// poll for response
	responseKey := pairingResponseKey(code)
	deadline := time.Now().Add(PairingTimeout)

	for time.Now().Before(deadline) {
		time.Sleep(PollInterval)
		fmt.Print(".")

		responseRecord, err := d.LookupValue(responseKey, "")
		if err != nil || responseRecord == nil {
			continue
		}

		// got a response — parse it
		contact, err := parsePairingResponse(responseRecord)
		if err != nil {
			fmt.Println("\nWarning: received malformed response, ignoring")
			continue
		}

		fmt.Printf("\n\nPaired with %s (%s)\n", contact.Name, contact.Address)
		return contact, nil
	}

	fmt.Println("\nPairing timed out. The code has expired.")
	return nil, fmt.Errorf("pairing timed out")
}

// Join completes a pairing session as the joiner
// looks up the code in DHT, responds, returns the initiator's contact
func Join(
	d *dht.DHT,
	name string,
	address string,
	privKey ed25519.PrivateKey,
	code string,
) (*Contact, error) {
	fmt.Printf("Looking up pairing code %s...\n", code)

	// look up initiator's record
	initiatorRecord, err := d.LookupValue(code, "")
	if err != nil {
		return nil, fmt.Errorf("lookup failed: %w", err)
	}
	if initiatorRecord == nil {
		return nil, fmt.Errorf("pairing code %s not found — check the code and try again", code)
	}

	// parse initiator's info
	initiator, err := parsePairingResponse(initiatorRecord)
	if err != nil {
		return nil, fmt.Errorf("invalid pairing record: %w", err)
	}

	fmt.Printf("Found %s (%s)\n", initiator.Name, initiator.Address)
	fmt.Println("Sending response...")

	// create and announce our response record
	responseKey := pairingResponseKey(code)
	responseRecord, err := createPairingRecord(name, address, privKey, responseKey, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create response record: %w", err)
	}

	if err := d.Announce(responseRecord); err != nil {
		return nil, fmt.Errorf("failed to announce response: %w", err)
	}

	fmt.Printf("Paired with %s (%s)\n", initiator.Name, initiator.Address)
	return initiator, nil
}

// pairingKey returns the DHT key for an initiator pairing record
func pairingKey(code string) string {
	return "pair:" + code
}

// pairingResponseKey returns the DHT key for a response record
func pairingResponseKey(code string) string {
	return "pair:" + code + ":response"
}

// createPairingRecord creates a signed DHT record for pairing
func createPairingRecord(
	name string,
	address string,
	privKey ed25519.PrivateKey,
	recordName string,
	isResponse bool,
) (dht.Record, error) {
	pr := PairingRecord{
		Name:       name,
		Address:    address,
		PublicKey:  hex.EncodeToString(privKey.Public().(ed25519.PublicKey)),
		Code:       recordName,
		IsResponse: isResponse,
	}

	serviceData, err := json.Marshal(pr)
	if err != nil {
		return dht.Record{}, fmt.Errorf("failed to encode pairing data: %w", err)
	}

	return dht.CreateRecord(dht.RegisterOptions{
		Name:       recordName,
		Address:    address,
		Services:   []string{"pairing:" + string(serviceData)},
		GroupKey:   "",
		PrivateKey: privKey,
		TTL:        PairingTTL,
	})
}

// parsePairingResponse extracts contact info from a DHT record
func parsePairingResponse(record *dht.Record) (*Contact, error) {
	// pairing data is encoded in Services[0] as "pairing:<json>"
	if len(record.Services) == 0 {
		// fallback — use raw record fields
		return &Contact{
			Name:      record.Name,
			Address:   record.Address,
			PublicKey: record.PublicKey,
			PairedAt:  time.Now(),
		}, nil
	}

	const prefix = "pairing:"
	svc := record.Services[0]
	if len(svc) < len(prefix) {
		return nil, fmt.Errorf("invalid pairing service field")
	}

	var pr PairingRecord
	if err := json.Unmarshal([]byte(svc[len(prefix):]), &pr); err != nil {
		// fallback to raw fields
		return &Contact{
			Name:      record.Name,
			Address:   record.Address,
			PublicKey: record.PublicKey,
			PairedAt:  time.Now(),
		}, nil
	}

	return &Contact{
		Name:      pr.Name,
		Address:   pr.Address,
		PublicKey: pr.PublicKey,
		PairedAt:  time.Now(),
	}, nil
}
