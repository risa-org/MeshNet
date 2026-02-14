package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"meshnet/core"
	"meshnet/dht"
)

func main() {
	fmt.Println("Meshnet Starting...")

	node := core.NewNode()
	err := node.Start()
	if err != nil {
		fmt.Println("Failed to start node:", err)
		os.Exit(1)
	}

	fmt.Println("Node Started. My Address:", node.Address())
	fmt.Println("Public Key:", node.PublicKey())

	fmt.Println("Connecting to peers....")
	node.Bootstrap()
	time.Sleep(3 * time.Second)
	fmt.Println("Bootstrap complete. Node is live on mesh.")

	selfID, err := dht.NodeIDFromHex(node.PublicKey())
	if err != nil {
		fmt.Println("Failed to parse node ID:", err)
		os.Exit(1)
	}

	portStr := os.Getenv("PORT")
	port := 9001
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	d := dht.New(node.Address(), selfID, port)
	err = d.Start()
	if err != nil {
		fmt.Println("Failed to start DHT:", err)
		os.Exit(1)
	}

	peer := os.Getenv("PEER")
	if peer != "" {
		err := d.PingPeer(peer)
		if err != nil {
			fmt.Println("Could not ping peer:", err)
		}
	}

	name := os.Getenv("NAME")
	if name == "" {
		name = "node-" + node.PublicKey()[:8]
	}

	record, err := dht.CreateRecord(dht.RegisterOptions{
		Name:       name,
		Address:    node.Address(),
		Services:   []string{},
		GroupKey:   "",
		PrivateKey: node.PrivateKey(),
	})
	if err != nil {
		fmt.Println("Failed to create record:", err)
		os.Exit(1)
	}

	time.Sleep(2 * time.Second)

	err = d.Announce(record)
	if err != nil {
		fmt.Println("Failed to announce:", err)
	} else {
		fmt.Printf("Announced as %q on the mesh\n", name)
	}

	// debug — check routing table size
	time.Sleep(1 * time.Second)
	fmt.Printf("Routing table size before lookup: %d\n", d.TableSize())
	lookupName := os.Getenv("LOOKUP")
	if lookupName != "" {
		go func() {
			time.Sleep(5 * time.Second)
			fmt.Printf("Looking up %q on the mesh...\n", lookupName)
			record, err := d.LookupValue(lookupName, "")
			if err != nil {
				fmt.Printf("Lookup error: %v\n", err)
				return
			}
			if record == nil {
				fmt.Printf("No record found for %q\n", lookupName)
				return
			}
			fmt.Printf("Found %q → %s\n", record.Name, record.Address)
			if len(record.Services) > 0 {
				fmt.Printf("  Services: %v\n", record.Services)
			}
		}()
	}

	fmt.Println("Meshnet running. Press Ctrl+C to stop.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down...")
	d.Stop()
	node.Stop()
}
