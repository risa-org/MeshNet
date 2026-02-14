package dht

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const RecordTTL = time.Hour

type Record struct {
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	PublicKey string   `json:"public_key"`
	Services  []string `json:"services"`
	GroupKey  string   `json:"group_key"`
	Signature string   `json:"signature"`
	Expires   int64    `json:"expires"`
}

func (r *Record) IsExpired() bool {
	return time.Now().After(time.Unix(r.Expires, 0))
}

func (r *Record) IsPublic() bool {
	return r.GroupKey == ""
}

func (r *Record) SigningPayload() []byte {
	payload, _ := json.Marshal(struct {
		Name      string   `json:"name"`
		Address   string   `json:"address"`
		PublicKey string   `json:"public_key"`
		Services  []string `json:"services"`
		GroupKey  string   `json:"group_key"`
		Expires   int64    `json:"expires"`
	}{
		Name:      r.Name,
		Address:   r.Address,
		PublicKey: r.PublicKey,
		Services:  r.Services,
		GroupKey:  r.GroupKey,
		Expires:   r.Expires,
	})

	hash := sha256.Sum256(payload)
	return hash[:]
}

func (r *Record) Verify() error {
	pubKeyBytes, err := hex.DecodeString(r.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	sigBytes, err := hex.DecodeString(r.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	payload := r.SigningPayload()
	if !ed25519.Verify(pubKey, payload, sigBytes) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func RecordID(name string) NodeID {
	hash := sha256.Sum256([]byte(name))
	var id NodeID
	copy(id[:], hash[:])
	return id
}

type Store struct {
	records map[string]Record
	mu      sync.RWMutex
	done    chan struct{}
}

func NewStore() *Store {
	return &Store{
		records: make(map[string]Record),
		done:    make(chan struct{}),
	}
}

func (s *Store) Start() {
	go s.cleanupLoop()
}

func (s *Store) Stop() {
	close(s.done)
}

func (s *Store) Put(r Record) error {
	if r.IsExpired() {
		return fmt.Errorf("record is already expired")
	}
	if err := r.Verify(); err != nil {
		return fmt.Errorf("invalid record: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.records[r.Name]
	if exists {
		if existing.PublicKey != r.PublicKey {
			return fmt.Errorf("name %q is owned by a different key", r.Name)
		}
	}
	s.records[r.Name] = r
	return nil
}

func (s *Store) Get(name string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, exists := s.records[name]
	if !exists {
		return Record{}, false
	}
	if r.IsExpired() {
		return Record{}, false
	}
	return r, true
}

func (s *Store) GetPublic(name string) (Record, bool) {
	r, exists := s.Get(name)
	if !exists {
		return Record{}, false
	}
	if !r.IsPublic() {
		return Record{}, false
	}
	return r, true
}

func (s *Store) GetForGroup(name string, groupKey string) (Record, bool) {
	r, exists := s.Get(name)
	if !exists {
		return Record{}, false
	}
	if r.GroupKey != groupKey {
		return Record{}, false
	}
	return r, true
}

func (s *Store) Delete(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, name)
}

func (s *Store) All() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Record
	for _, r := range s.records {
		if !r.IsExpired() {
			result = append(result, r)
		}
	}
	return result
}

func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.done:
			return
		}
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for name, r := range s.records {
		if r.IsExpired() {
			delete(s.records, name)
			removed++
		}
	}
	if removed > 0 {
		fmt.Printf("DHT store: removed %d expired records \n", removed)
	}
}
