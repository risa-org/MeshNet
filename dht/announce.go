package dht

import (
	"fmt"
	"time"
)

// reannounceInterval is how often we re-announce our record
// must be less than RecordTTL (1 hour) to prevent expiry
// 45 minutes gives a 15 minute safety margin
const reannounceInterval = 45 * time.Minute

// Reannouncer manages periodic re-announcement of a record
type Reannouncer struct {
	dht    *DHT
	record Record
	done   chan struct{}
}

// NewReannouncer creates a reannouncer for a record
func NewReannouncer(d *DHT, record Record) *Reannouncer {
	return &Reannouncer{
		dht:    d,
		record: record,
		done:   make(chan struct{}),
	}
}

// Start launches the re-announcement loop in the background
func (r *Reannouncer) Start() {
	go r.loop()
}

// Stop shuts down the re-announcement loop
func (r *Reannouncer) Stop() {
	close(r.done)
}

// UpdateRecord replaces the record being announced
// call this if your services change
func (r *Reannouncer) UpdateRecord(record Record) {
	r.record = record
}

func (r *Reannouncer) loop() {
	ticker := time.NewTicker(reannounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Printf("Re-announcing %q on the mesh...\n", r.record.Name)
			err := r.dht.Announce(r.record)
			if err != nil {
				fmt.Println("Re-announce failed:", err)
			}
		case <-r.done:
			return
		}
	}
}