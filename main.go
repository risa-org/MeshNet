package main

import (
	"fmt"
	"meshnet/core"
	"meshnet/dht"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	d := dht.New(node.Address())
	err = d.Start()
	if err != nil {
		fmt.Println("Failed to start DHT:", err)
		os.Exit(1)
	}

	fmt.Println("Meshnet running. Press Ctrl+C to stop.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down...")
	d.Stop()
	node.Stop()
}
