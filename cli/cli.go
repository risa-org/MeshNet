package cli

import (
	"crypto/ed25519"
	"encoding/hex"
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
	"meshnet/pairing"
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
	case "pair":
		cmdPair(os.Args[2:])
	case "contacts":
		cmdContacts(os.Args[2:])
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
  pair      Pair with another device
  contacts  List paired devices
  status    Show this node's status
  peers     List known DHT peers
  peer      Manage peers (add/remove/list)
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

	contacts, err := pairing.LoadContacts()
	if err == nil {
		if c := contacts.FindByName(name); c != nil {
			fmt.Printf("\nFound in contacts: %s\n", name)
			fmt.Printf("  Address:  %s\n", c.Address)
			fmt.Printf("  Paired:   %s ago\n", time.Since(c.PairedAt).Round(time.Minute))
			return
		}
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

func cmdPair(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println(`Pair with another MeshNet device

USAGE:
  meshnet pair              Generate a pairing code (you share it)
  meshnet pair MESH-XXXX    Join using a code from another device

EXAMPLES:
  meshnet pair
  meshnet pair MESH-4729`)
	}
	fs.Parse(args)

	if !dht.IsNodeRunning() {
		fmt.Println("Error: no MeshNet node is running.")
		fmt.Println("Start one first with: meshnet start")
		os.Exit(1)
	}

	// get our own info from the running node
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", dht.APIPort))
	if err != nil {
		fmt.Println("Failed to reach running node:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		fmt.Println("Failed to read node status:", err)
		os.Exit(1)
	}

	nodeName := fmt.Sprintf("%v", status["name"])
	nodeAddress := fmt.Sprintf("%v", status["address"])

	// we need private key for signing — read from identity file
	privKey, err := loadPrivateKey()
	if err != nil {
		fmt.Println("Failed to load private key:", err)
		os.Exit(1)
	}

	// load or create contact book
	contacts, err := pairing.LoadContacts()
	if err != nil {
		fmt.Println("Failed to load contacts:", err)
		os.Exit(1)
	}

	// start a minimal DHT client for pairing operations
	d, cleanup, err := startPairingDHT(nodeAddress, privKey)
	if err != nil {
		fmt.Println("Failed to connect to DHT:", err)
		os.Exit(1)
	}
	defer cleanup()

	var contact *pairing.Contact

	if fs.NArg() == 0 {
		// no code provided — we are the initiator
		contact, err = pairing.Initiate(d, nodeName, nodeAddress, privKey)
	} else {
		// code provided — we are the joiner
		code := fs.Arg(0)
		contact, err = pairing.Join(d, nodeName, nodeAddress, privKey, code)
	}

	if err != nil {
		fmt.Println("Pairing failed:", err)
		os.Exit(1)
	}

	// save to contacts
	contacts.Add(*contact)
	if err := contacts.Save(); err != nil {
		fmt.Println("Warning: could not save contact:", err)
	} else {
		fmt.Printf("Saved %s to contacts.\n", contact.Name)
	}
}

// loadPrivateKey reads the private key from identity.json
func loadPrivateKey() (ed25519.PrivateKey, error) {
	data, err := os.ReadFile("identity.json")
	if err != nil {
		return nil, fmt.Errorf("could not read identity.json: %w", err)
	}

	var identity struct {
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("could not parse identity.json: %w", err)
	}

	keyBytes, err := hex.DecodeString(identity.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	return ed25519.PrivateKey(keyBytes), nil
}

// startPairingDHT creates a minimal DHT instance on a temporary port
// for use by the pair command — separate from the running node's DHT
func startPairingDHT(address string, privKey ed25519.PrivateKey) (*dht.DHT, func(), error) {
	pubKey := privKey.Public().(ed25519.PublicKey)
	selfID, err := dht.NodeIDFromBytes(pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create node ID: %w", err)
	}

	// use port 9010 for pairing — avoids conflict with main DHT on 9002
	d := dht.New(address, selfID, 9010)
	if err := d.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start pairing DHT: %w", err)
	}

	// bootstrap from the running node's API — it knows peers
	client := &http.Client{Timeout: 5 * time.Second}
	peersResp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/peers", dht.APIPort))
	if err == nil {
		defer peersResp.Body.Close()
		var peers []dht.PeerInfo
		if json.NewDecoder(peersResp.Body).Decode(&peers) == nil {
			for _, p := range peers {
				addr := fmt.Sprintf("[%s]:%d", p.Addr, p.Port)
				d.PingPeer(addr)
			}
		}
	}

	cleanup := func() {
		d.Stop()
	}

	return d, cleanup, nil
}

func cmdContacts(args []string) {
	fs := flag.NewFlagSet("contacts", flag.ExitOnError)
	fs.Parse(args)

	contacts, err := pairing.LoadContacts()
	if err != nil {
		fmt.Println("Failed to load contacts:", err)
		os.Exit(1)
	}

	all := contacts.All()
	if len(all) == 0 {
		fmt.Println("No contacts yet.")
		fmt.Println("Pair with another device using: meshnet pair")
		return
	}

	fmt.Printf("\nContacts (%d)\n", len(all))
	fmt.Println("──────────────────────────────────────────────────")
	for _, c := range all {
		fmt.Printf("  %-20s  %s\n", c.Name, c.Address)
		fmt.Printf("  %-20s  paired %s ago\n", "",
			time.Since(c.PairedAt).Round(time.Minute))
	}
	fmt.Println("──────────────────────────────────────────────────")
}
