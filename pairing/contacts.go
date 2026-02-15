package pairing

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const contactsFile = "contacts.json"

// Contact represents a paired device
type Contact struct {
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	PublicKey string    `json:"public_key"`
	PairedAt  time.Time `json:"paired_at"`
}

// ContactBook manages the local list of paired devices
type ContactBook struct {
	mu       sync.RWMutex
	contacts map[string]Contact // keyed by public key
}

// LoadContacts reads contacts from disk
// returns empty book if file doesn't exist
func LoadContacts() (*ContactBook, error) {
	book := &ContactBook{
		contacts: make(map[string]Contact),
	}

	data, err := os.ReadFile(contactsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return book, nil
		}
		return nil, fmt.Errorf("failed to read contacts: %w", err)
	}

	var list []Contact
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse contacts: %w", err)
	}

	for _, c := range list {
		book.contacts[c.PublicKey] = c
	}

	return book, nil
}

// Save writes contacts to disk
func (b *ContactBook) Save() error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var list []Contact
	for _, c := range b.contacts {
		list = append(list, c)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode contacts: %w", err)
	}

	return os.WriteFile(contactsFile, data, 0600)
}

// Add adds or updates a contact
func (b *ContactBook) Add(c Contact) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.contacts[c.PublicKey] = c
}

// All returns all contacts
func (b *ContactBook) All() []Contact {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var list []Contact
	for _, c := range b.contacts {
		list = append(list, c)
	}
	return list
}

// FindByName finds a contact by name
func (b *ContactBook) FindByName(name string) *Contact {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.contacts {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

// FindByAddress finds a contact by Yggdrasil address
func (b *ContactBook) FindByAddress(addr string) *Contact {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.contacts {
		if c.Address == addr {
			return &c
		}
	}
	return nil
}
