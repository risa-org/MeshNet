package dht

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// peersFile is where we save known peers between sessions
const peersFile = "peers.json"

// savedPeer is the serializable form of a Contact
// net.IP doesn't serialize cleanly so we use strings
type savedPeer struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

// SavePeers writes all known contacts to disk
// called on shutdown so next startup doesn't start blind
func (d *DHT) SavePeers() error {
	contacts := d.table.All()
	if len(contacts) == 0 {
		return nil
	}

	var peers []savedPeer
	for _, c := range contacts {
		peers = append(peers, savedPeer{
			ID:   c.ID.String(),
			Addr: c.Address.String(),
			Port: c.Port,
		})
	}

	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode peers: %w", err)
	}

	if err := os.WriteFile(peersFile, data, 0644); err != nil {
		return fmt.Errorf("failed to save peers: %w", err)
	}

	fmt.Printf("Saved %d peers to disk\n", len(peers))
	return nil
}

// LoadPeers reads saved peers from disk and pings each one
// peers that don't respond are skipped — they may be offline
// peers that respond are added to the routing table
func (d *DHT) LoadPeers() {
	data, err := os.ReadFile(peersFile)
	if err != nil {
		// no peers file yet — first run or file was deleted
		return
	}

	var peers []savedPeer
	if err := json.Unmarshal(data, &peers); err != nil {
		fmt.Println("Warning: could not parse peers file:", err)
		return
	}

	if len(peers) == 0 {
		return
	}

	fmt.Printf("Loading %d saved peers...\n", len(peers))

	// ping each saved peer concurrently
	// only add ones that actually respond
	var wg sync.WaitGroup
	alive := 0
	var mu sync.Mutex

	for _, p := range peers {
		wg.Add(1)
		go func(sp savedPeer) {
			defer wg.Done()
			addr := fmt.Sprintf("[%s]:%d", sp.Addr, sp.Port)
			err := d.PingPeer(addr)
			if err == nil {
				mu.Lock()
				alive++
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	fmt.Printf("Restored %d/%d peers from disk\n", alive, len(peers))
}

// BootstrapDHT attempts to populate the routing table using
// saved peers first, then well-known bootstrap nodes
// returns number of peers successfully contacted
func (d *DHT) BootstrapDHT() int {
	// try saved peers first — they're more likely to be MeshNet nodes
	d.LoadPeers()

	if d.table.Size() > 0 {
		return d.table.Size()
	}

	// no saved peers — try well-known bootstrap nodes
	fmt.Println("No saved peers found. Trying bootstrap nodes...")
	contacted := 0

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, addr := range meshnetBootstrapPeers {
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			err := d.PingPeer(a)
			if err == nil {
				mu.Lock()
				contacted++
				mu.Unlock()
				fmt.Printf("Bootstrap peer alive: %s\n", a)
			}
		}(addr)
	}

	wg.Wait()

	if contacted == 0 {
		fmt.Println("No bootstrap peers reachable — DHT starting in isolated mode")
		fmt.Println("Use 'meshnet peer add <address>' to connect to known nodes")
	}

	return d.table.Size()
}

// All returns all contacts in the routing table
// added to RoutingTable so SavePeers can access them
func (rt *RoutingTable) All() []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var all []Contact
	for _, bucket := range rt.buckets {
		all = append(all, bucket...)
	}
	return all
}

// PeerInfo holds displayable info about a known peer
type PeerInfo struct {
	ID      string
	Addr    string
	Port    int
	Latency time.Duration
	Alive   bool
}

// PingAllPeers pings all known peers and returns their status
// useful for the 'peers' CLI command
func (d *DHT) PingAllPeers() []PeerInfo {
	contacts := d.table.All()
	results := make([]PeerInfo, len(contacts))

	var wg sync.WaitGroup
	for i, c := range contacts {
		wg.Add(1)
		go func(idx int, contact Contact) {
			defer wg.Done()
			addr := fmt.Sprintf("[%s]:%d", contact.Address.String(), contact.Port)

			start := time.Now()
			self := Contact{
				ID:      d.table.self,
				Address: net.ParseIP(d.address),
				Port:    d.port,
			}
			_, err := SendPing(addr, self)
			latency := time.Since(start)

			results[idx] = PeerInfo{
				ID:      contact.ID.String()[:16],
				Addr:    contact.Address.String(),
				Port:    contact.Port,
				Latency: latency,
				Alive:   err == nil,
			}
		}(i, c)
	}

	wg.Wait()
	return results
}
