package dht

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// APIPort is the local HTTP API port
// only listens on localhost — not exposed to network
const APIPort = 9099

// StartAPI launches a local HTTP API for CLI commands to talk to
// listens only on localhost so it's never exposed to the mesh
func (d *DHT) StartAPI(nodeName string, nodeAddress string, nodePublicKey string) {
	mux := http.NewServeMux()

	// GET /status — node identity and routing table info
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":       nodeName,
			"address":    nodeAddress,
			"public_key": nodePublicKey,
			"peers":      d.table.Size(),
			"records":    d.store.Size(),
		})
	})

	// GET /lookup?name=alice&group= — look up a name
	mux.HandleFunc("/lookup", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		group := r.URL.Query().Get("group")
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}

		record, err := d.LookupValue(name, group)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if record == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(record)
	})

	// GET /peers — list known peers
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		peers := d.PingAllPeers()
		json.NewEncoder(w).Encode(peers)
	})

	// POST /peer?addr=... — add a peer
	mux.HandleFunc("/peer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		addr := r.URL.Query().Get("addr")
		if addr == "" {
			http.Error(w, "addr required", http.StatusBadRequest)
			return
		}
		if err := d.PingPeer(addr); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		d.SavePeers()
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", APIPort),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// API failed to start — not fatal, just means CLI commands
			// won't work while node is running
			fmt.Println("Warning: local API failed:", err)
		}
	}()

	fmt.Printf("Local API running on http://127.0.0.1:%d\n", APIPort)
}

// IsNodeRunning checks if a node is already running by hitting the local API
func IsNodeRunning() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", APIPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
