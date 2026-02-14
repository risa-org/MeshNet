package dht

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const alpha = 3

type lookupState struct {
	target NodeID

	self NodeID

	contacted map[NodeID]bool

	candidates []Contact

	mu sync.Mutex
}

func newLookupState(self NodeID, target NodeID, seeds []Contact) *lookupState {
	ls := &lookupState{
		target:    target,
		self:      self,
		contacted: make(map[NodeID]bool),
	}
	ls.candidates = append(ls.candidates, seeds...)
	return ls
}

func (ls *lookupState) addCandidates(contacts []Contact) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	for _, c := range contacts {
		if c.ID == ls.self {
			continue
		}
		if ls.contacted[c.ID] {
			continue
		}
		duplicate := false
		for _, existing := range ls.candidates {
			if existing.ID == c.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			ls.candidates = append(ls.candidates, c)
		}
	}

	for i := 1; i < len(ls.candidates); i++ {
		for j := i; j > 0; j-- {
			if ls.candidates[j].ID.Less(ls.candidates[j-1].ID, ls.target) {
				ls.candidates[j], ls.candidates[j-1] =
					ls.candidates[j-1], ls.candidates[j]
			}
		}
	}
}

func (ls *lookupState) nextBatch() []Contact {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	var batch []Contact
	for _, c := range ls.candidates {
		if !ls.contacted[c.ID] {
			batch = append(batch, c)
			if len(batch) >= alpha {
				break
			}
		}
	}
	return batch
}

func (ls *lookupState) markContacted(id NodeID) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.contacted[id] = true
}

func (ls *lookupState) closest(k int) []Contact {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if k > len(ls.candidates) {
		k = len(ls.candidates)
	}
	return ls.candidates[:k]
}

func (d *DHT) LookupNode(target NodeID) []Contact {
	seeds := d.table.Closest(target, K)
	if len(seeds) == 0 {
		return nil
	}

	state := newLookupState(d.table.self, target, seeds)

	for {
		batch := state.nextBatch()
		if len(batch) == 0 {
			break
		}

		var wg sync.WaitGroup
		results := make(chan []Contact, len(batch))

		for _, contact := range batch {
			wg.Add(1)
			go func(c Contact) {
				defer wg.Done()
				state.markContacted(c.ID)

				contacts, err := SendFindNode(c.Addr(), d.table.self, target)
				if err != nil {
					d.table.Remove(c.ID)
					return
				}

				var found []Contact
				for _, ci := range contacts {
					id, err := NodeIDFromHex(ci.ID)
					if err != nil {
						continue
					}
					ip := net.ParseIP(ci.Addr)
					if ip == nil {
						continue
					}
					found = append(found, Contact{
						ID:      id,
						Address: ip,
						Port:    ci.Port,
					})
				}
				results <- found
			}(contact)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		for contacts := range results {
			state.addCandidates(contacts)
			for _, c := range contacts {
				d.table.Add(c)
			}
		}
	}

	return state.closest(K)
}

func (d *DHT) LookupValue(name string, groupKey string) (*Record, error) {
	target := RecordID(name)

	seeds := d.table.Closest(target, K)
	if len(seeds) == 0 {
		return nil, fmt.Errorf("no known nodes to query")
	}

	state := newLookupState(d.table.self, target, seeds)

	for {
		batch := state.nextBatch()
		if len(batch) == 0 {
			break
		}

		found := make(chan *Record, 1)
		var wg sync.WaitGroup

		for _, contact := range batch {
			wg.Add(1)
			go func(c Contact) {
				defer wg.Done()
				state.markContacted(c.ID)

				record, closer, err := SendFindValue(
					c.Addr(),
					d.table.self,
					name,
					groupKey,
				)
				if err != nil {
					d.table.Remove(c.ID)
					return
				}

				if record != nil {
					select {
					case found <- record:
					default:
					}
					return
				}

				if closer != nil {
					var contacts []Contact
					for _, ci := range closer {
						id, err := NodeIDFromHex(ci.ID)
						if err != nil {
							continue
						}
						ip := net.ParseIP(ci.Addr)
						if ip == nil {
							continue
						}
						contacts = append(contacts, Contact{
							ID:      id,
							Address: ip,
							Port:    ci.Port,
						})
					}
					state.addCandidates(contacts)
				}
			}(contact)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case record := <-found:
			return record, nil
		case <-done:
		case <-time.After(readTimeout):
			return nil, nil
		}
	}

	return nil, nil
}

func (d *DHT) Announce(record Record) error {
	if err := record.Verify(); err != nil {
		return fmt.Errorf("invalid record: %w", err)
	}

	target := RecordID(record.Name)
	closest := d.LookupNode(target)

	if len(closest) == 0 {
		return d.store.Put(record)
	}

	var wg sync.WaitGroup
	stored := 0
	var mu sync.Mutex

	for _, contact := range closest {
		wg.Add(1)
		go func(c Contact) {
			defer wg.Done()
			err := SendStore(c.Addr(), record)
			if err == nil {
				mu.Lock()
				stored++
				mu.Unlock()
			}
		}(contact)
	}

	wg.Wait()

	d.store.Put(record)

	fmt.Printf("DHT: announced %q on %d nodes\n", record.Name, stored)
	return nil
}
