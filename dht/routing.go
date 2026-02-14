package dht

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"sync"
)

const K = 20

type NodeID [32]byte

func NodeIDFromPublicKey(pub ed25519.PublicKey) NodeID {
	var id NodeID
	copy(id[:], pub[:32])
	return id
}

func NodeIDFromHex(s string) (NodeID, error) {
	var id NodeID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, fmt.Errorf("invalid node ID hex: %w", err)
	}
	copy(id[:], b)
	return id, nil
}

func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

func (id NodeID) XOR(other NodeID) NodeID {
	var result NodeID
	for i := 0; i < 32; i++ {
		result[i] = id[i] ^ other[i]
	}
	return result
}

func (id NodeID) Less(other NodeID, target NodeID) bool {
	distA := id.XOR(target)
	distB := other.XOR(target)
	for i := 0; i < 32; i++ {
		if distA[i] != distB[i] {
			return distA[i] < distB[i]
		}
	}
	return false
}

func (id NodeID) bucketIndex(other NodeID) int {
	xor := id.XOR(other)
	for i := 0; i < 32; i++ {
		if xor[i] != 0 {
			b := xor[i]
			bit := 0
			for b&0x80 == 0 {
				b <<= 1
				bit++
			}
			return i*8 + bit
		}
	}
	return -1
}

type Contact struct {
	ID      NodeID
	Address net.IP
	Port    int
}

func (c Contact) Addr() string {
	return fmt.Sprintf("[%s]:%d", c.Address.String(), c.Port)
}

type RoutingTable struct {
	self    NodeID
	buckets [256][]Contact
	mu      sync.RWMutex
}

func NewRoutingTable(self NodeID) *RoutingTable {
	return &RoutingTable{
		self: self,
	}
}

func (rt *RoutingTable) Add(c Contact) {
	if c.ID == rt.self {
		fmt.Println("DEBUG Add: skipping self")
		return
	}

	idx := rt.self.bucketIndex(c.ID)
	fmt.Printf("DEBUG Add: id=%s idx=%d\n", c.ID.String()[:8], idx)
	if idx < 0 {
		fmt.Println("DEBUG Add: negative index, skipping")
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	bucket := rt.buckets[idx]
	fmt.Printf("DEBUG Add: bucket %d has %d entries\n", idx, len(bucket))
	for i, existing := range bucket {
		if existing.ID == c.ID {
			rt.buckets[idx] = append(
				append(bucket[:i], bucket[i+1:]...),
				c,
			)
			return
		}
	}

	if len(bucket) < K {
		rt.buckets[idx] = append(bucket, c)
		return
	}
}

func (rt *RoutingTable) Remove(id NodeID) {
	idx := rt.self.bucketIndex(id)
	if idx < 0 {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	bucket := rt.buckets[idx]
	for i, c := range bucket {
		if c.ID == id {
			rt.buckets[idx] = append(bucket[:i], bucket[i+1:]...)
			return
		}
	}
}

func (rt *RoutingTable) Closest(target NodeID, count int) []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var all []Contact
	for _, bucket := range rt.buckets {
		all = append(all, bucket...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ID.Less(all[j].ID, target)
	})

	if count > len(all) {
		count = len(all)
	}
	return all[:count]
}

func (rt *RoutingTable) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	total := 0
	for _, bucket := range rt.buckets {
		total += len(bucket)
	}
	return total
}
