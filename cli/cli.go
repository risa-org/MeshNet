package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"meshnet/core"
	"meshnet/dht"
)

// Run is the entry point for the CLI
// parses the command and dispatches to the right handler
func Run() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	command := os.Args[1]

	switch command {
	case "start":
		cmdStart(os.Args[2:])
	case "lookup":
		cmdLookup(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "peers":
		cmdPeers(os.Args[2:])
	case "peer":
		cmdPeer(os.Args[2:])
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`MeshNet — decentralized peer-to-peer network

USAGE:
  meshnet <command> [flags]

COMMANDS:
  start     Start the MeshNet node
  lookup    Look up a name on the mesh
  status    Show this node's status and identity
  peers     List known peers
  peer      Manage peers (add/remove)
  help      Show this help

Run 'meshnet <command> --help' for command-specific flags.`)
}

// ── start ────────────────────────────────────────────────────────────────────

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	name := fs.String("name", "", "Name to register on the mesh (default: node-<pubkey>)")
	port := fs.Int("port", 9001, "DHT listen port")
	identity := fs.String("identity", "identity.json", "Path to identity file")
	peer := fs.String("peer", "", "Bootstrap peer address e.g. [::1]:9002")
	services := fs.String("services", "", "Comma-separated services e.g. ssh:22,http:80")
	tun := fs.Bool("tun", false, "Create TUN interface for browser/OS access (requires admin)")
	yggBin := fs.String("yggdrasil", "bin/yggdrasil.exe", "Path to yggdrasil binary")
	fs.Usage = func() {
		fmt.Println(`Start the MeshNet node

USAGE:
  meshnet start [flags]

FLAGS:`)
		fs.PrintDefaults()
		fmt.Println(`
EXAMPLES:
  meshnet start
  meshnet start --name alice
  meshnet start --name myserver --services ssh:22,http:80
  meshnet start --port 9002 --identity identity2.json
  meshnet start --peer "[::1]:9002"`)
	}
	fs.Parse(args)

	// set identity path via env so core package picks it up
	os.Setenv("IDENTITY", *identity)

	fmt.Println("MeshNet Starting...")

	// start yggdrasil node
	node := core.NewNode()
	if err := node.Start(); err != nil {
		fmt.Println("Failed to start node:", err)
		os.Exit(1)
	}

	fmt.Println("Address:", node.Address())
	fmt.Println("Public Key:", node.PublicKey())

	// connect to yggdrasil bootstrap peers
	fmt.Println("Connecting to Yggdrasil peers...")
	node.Bootstrap()
	time.Sleep(3 * time.Second)
	fmt.Println("Connected to Yggdrasil mesh.")

	// set up DHT
	selfID, err := dht.NodeIDFromHex(node.PublicKey())
	if err != nil {
		fmt.Println("Failed to parse node ID:", err)
		os.Exit(1)
	}

	d := dht.New(node.Address(), selfID, *port)
	if err := d.Start(); err != nil {
		fmt.Println("Failed to start DHT:", err)
		os.Exit(1)
	}

	// TUN mode — start Yggdrasil subprocess for OS-level mesh access
	var yggSvc *core.YggService
	if *tun {
		fmt.Println("\nTUN mode enabled — starting Yggdrasil for OS integration...")

		yggSvc = core.NewYggService(*yggBin)

		// only write config if installed service doesn't exist
		if !yggSvc.IsInstalled() {
			if err := yggSvc.WriteConfig(core.PrivKeyHex(node.PrivateKey())); err != nil {
				fmt.Println("Failed to write Yggdrasil config:", err)
				yggSvc = nil
			}
		}

		if yggSvc != nil {
			if err := yggSvc.Start(); err != nil {
				fmt.Println("Failed to start Yggdrasil TUN:", err)
				fmt.Println("Try: Run PowerShell as Administrator")
				yggSvc = nil
			} else {
				// wait for TUN routing to fully stabilize before proceeding
				// the adapter is created but routes take a moment to activate
				fmt.Println("Waiting for TUN routing to stabilize...")
				time.Sleep(5 * time.Second)
				if tunAddr, err := yggSvc.GetAddress(); err == nil {
					fmt.Printf("TUN interface active — mesh address: %s\n", tunAddr)
					fmt.Println("Browser can now reach Yggdrasil addresses directly")
				}
			}
		}
	}

	// bootstrap DHT routing table
	// tries saved peers first, then well-known bootstrap nodes
	d.BootstrapDHT()

	// manual peer override
	if *peer != "" {
		if err := d.PingPeer(*peer); err != nil {
			fmt.Println("Could not reach peer:", err)
		}
	}

	// build our name
	nodeName := *name
	if nodeName == "" {
		nodeName = "node-" + node.PublicKey()[:8]
	}

	d.StartAPI(nodeName, node.Address(), node.PublicKey())

	// parse services
	var serviceList []string
	if *services != "" {
		// split on comma manually — no strings import needed
		start := 0
		for i := 0; i <= len(*services); i++ {
			if i == len(*services) || (*services)[i] == ',' {
				s := (*services)[start:i]
				if s != "" {
					serviceList = append(serviceList, s)
				}
				start = i + 1
			}
		}
	}

	// create and announce our record
	record, err := dht.CreateRecord(dht.RegisterOptions{
		Name:       nodeName,
		Address:    node.Address(),
		Services:   serviceList,
		GroupKey:   "",
		PrivateKey: node.PrivateKey(),
	})
	if err != nil {
		fmt.Println("Failed to create record:", err)
		os.Exit(1)
	}

	time.Sleep(1 * time.Second)

	if err := d.Announce(record); err != nil {
		fmt.Println("Failed to announce:", err)
	} else {
		fmt.Printf("Announced as %q on the mesh\n", nodeName)
		if len(serviceList) > 0 {
			fmt.Printf("Services: %v\n", serviceList)
		}
	}

	// start re-announcement so record never expires
	reannouncer := dht.NewReannouncer(d, record)
	reannouncer.Start()

	fmt.Println("\nMeshNet running. Press Ctrl+C to stop.")
	fmt.Printf("Other nodes can find you with: meshnet lookup %s\n\n", nodeName)

	// wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down...")
	reannouncer.Stop()

	// stop yggdrasil subprocess if we started it
	if yggSvc != nil {
		yggSvc.Stop()
	}

	d.SavePeers()
	d.Stop()
	node.Stop()
	fmt.Println("Goodbye.")
}

// ── lookup ───────────────────────────────────────────────────────────────────

func cmdLookup(args []string) {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	group := fs.String("group", "", "Group key for private record lookup")
	fs.Usage = func() {
		fmt.Println(`Look up a name on the MeshNet mesh

USAGE:
  meshnet lookup <name> [flags]

FLAGS:`)
		fs.PrintDefaults()
		fmt.Println(`
EXAMPLES:
  meshnet lookup alice
  meshnet lookup myserver`)
	}
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Println("Error: name required")
		fmt.Println("Usage: meshnet lookup <name>")
		os.Exit(1)
	}

	name := fs.Arg(0)

	if !dht.IsNodeRunning() {
		fmt.Println("Error: no MeshNet node is running.")
		fmt.Println("Start one first with: meshnet start")
		os.Exit(1)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/lookup?name=%s&group=%s",
		dht.APIPort, name, *group)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Println("Lookup failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Not found: %q is not registered on the mesh\n", name)
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Lookup error: status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var record dht.Record
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		fmt.Println("Failed to decode response:", err)
		os.Exit(1)
	}

	fmt.Printf("\nFound: %s\n", name)
	fmt.Printf("  Address:  %s\n", record.Address)
	fmt.Printf("  Owner:    %s...\n", record.PublicKey[:16])
	if len(record.Services) > 0 {
		fmt.Printf("  Services: %v\n", record.Services)
	}
	expires := time.Unix(record.Expires, 0)
	fmt.Printf("  Expires:  %s\n", time.Until(expires).Round(time.Minute))
}

// ── status ───────────────────────────────────────────────────────────────────

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println(`Show running node status

USAGE:
  meshnet status`)
	}
	fs.Parse(args)

	if !dht.IsNodeRunning() {
		fmt.Println("No MeshNet node is running.")
		fmt.Println("Start one with: meshnet start")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", dht.APIPort))
	if err != nil {
		fmt.Println("Failed to reach node:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		fmt.Println("Failed to decode status:", err)
		os.Exit(1)
	}

	fmt.Println("\nMeshNet Node Status")
	fmt.Println("───────────────────────────────────────")
	fmt.Printf("Name:       %v\n", status["name"])
	fmt.Printf("Address:    %v\n", status["address"])
	fmt.Printf("Public Key: %v\n", status["public_key"])
	fmt.Printf("Peers:      %v\n", status["peers"])
	fmt.Printf("Records:    %v\n", status["records"])
	fmt.Println("───────────────────────────────────────")
}

// ── peers ────────────────────────────────────────────────────────────────────

func cmdPeers(args []string) {
	fs := flag.NewFlagSet("peers", flag.ExitOnError)
	fs.Parse(args)

	if !dht.IsNodeRunning() {
		fmt.Println("No MeshNet node is running.")
		fmt.Println("Start one with: meshnet start")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/peers", dht.APIPort))
	if err != nil {
		fmt.Println("Failed to reach node:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var peers []dht.PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		fmt.Println("Failed to decode peers:", err)
		os.Exit(1)
	}

	if len(peers) == 0 {
		fmt.Println("No known peers.")
		return
	}

	fmt.Printf("\nKnown Peers (%d)\n", len(peers))
	fmt.Println("────────────────────────────────────────────────────")
	for _, p := range peers {
		status := "✓"
		latency := fmt.Sprintf("%v", p.Latency.Round(time.Millisecond))
		if !p.Alive {
			status = "✗"
			latency = "unreachable"
		}
		fmt.Printf("  %s  %s...  [%s]:%d  %s\n",
			status, p.ID[:12], p.Addr, p.Port, latency)
	}
}

// ── peer ─────────────────────────────────────────────────────────────────────

func cmdPeer(args []string) {
	if len(args) == 0 {
		fmt.Println(`Manage DHT peers

USAGE:
  meshnet peer add <address>    Add a peer manually
  meshnet peer list             List saved peers
  meshnet peer clear            Clear all saved peers`)
		return
	}

	switch args[0] {
	case "add":
		if len(args) < 2 {
			fmt.Println("Usage: meshnet peer add <address>")
			os.Exit(1)
		}
		peerAddr := args[1]

		if !dht.IsNodeRunning() {
			fmt.Println("No MeshNet node is running.")
			fmt.Println("Start one with: meshnet start")
			os.Exit(1)
		}

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Post(
			fmt.Sprintf("http://127.0.0.1:%d/peer?addr=%s", dht.APIPort, peerAddr),
			"", nil,
		)
		if err != nil {
			fmt.Println("Failed to reach node:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to add peer: status %d\n", resp.StatusCode)
			os.Exit(1)
		}
		fmt.Println("Peer added successfully.")

	case "list":
		data, err := os.ReadFile("peers.json")
		if err != nil {
			fmt.Println("No saved peers.")
			return
		}
		fmt.Println("Saved peers:")
		fmt.Println(string(data))

	case "clear":
		if err := os.Remove("peers.json"); err != nil {
			fmt.Println("No peers file to clear.")
			return
		}
		fmt.Println("Cleared all saved peers.")

	default:
		fmt.Printf("Unknown peer subcommand: %s\n", args[0])
		fmt.Println("Use: add, list, or clear")
		os.Exit(1)
	}
}
