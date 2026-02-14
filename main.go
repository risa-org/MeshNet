package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"meshnet/core"
)

func main() {
	fmt.Println("Meshnet Starting....")

	node := core.NewNode()
	err := node.Start()
	if err != nil {
		fmt.Println("Failed to start node:", err)
		os.Exit(1)
	}
	fmt.Println("Node started. My Address:", node.Address())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting Down...")
	node.Stop()
}
