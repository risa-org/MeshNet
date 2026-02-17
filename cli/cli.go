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
func Run() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	switch os.Args[1] {
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
	case "pair":
		cmdPair(os.Args[2:])
	case "contacts":
		cmdContacts(os.Args[2:])
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
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
  status    Show node status
  peers     List known DHT peers
  peer      Manage peers
  help      Show this help

Run 'meshnet <command> --help' for command-specific flags.`)
}

// ── start ────────────────────────────────────────────────────────────────────

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	name := fs.String("name", "", "Name to register on the mesh")
	port := fs.Int("port", 9001, "DHT listen port")
	identity := fs.String("identity", "identity.json", "Path to identity file")
	peer := fs.String("peer", "", "Bootstrap peer address e.g. [::1]:9002")
	services := fs.String("services", "", "Comma-separated services e.g. ssh:22,http:80")
	tun := fs.Bool("tun", false, "Enable TUN interface for browser/OS access (requires admin)")
	yggBin := fs.String("yggdrasil", "bin/yggdrasil.exe", "Path to yggdrasil binary")
	fs.Usage = func() {
		fmt.Println(`Start the MeshNet node

USAGE:
  meshnet start [flags]

FLAGS:`)
		fs.PrintDefaults()
		fmt.Println(`
EXAMPLES:
  meshnet start --name alice
  meshnet start --name alice --tun
  meshnet start --name myserver --services ssh:22,http:80`)
	}
	fs.Parse(args)

	os.Setenv("IDENTITY", *identity)

	fmt.Println("MeshNet Starting...")

	// ── identity + node ──────────────────────────────────────────────────────
	node := core.NewNode()
	if err := node.Start(); err != nil {
		fmt.Println("Failed to start node:", err)
		os.Exit(1)
	}

	fmt.Printf("Address:    %s\n", node.Address())
	fmt.Printf("Public Key: %s...\n\n", node.PublicKey()[:16])

	// ── mesh connection ──────────────────────────────────────────────────────
	var yggSvc *core.YggService

	if *tun {
		// TUN mode: subprocess handles ALL routing
		// do NOT connect embedded library to peers — causes routing conflict
		fmt.Print("Starting TUN interface")

		yggSvc = core.NewYggService(*yggBin)

		if !yggSvc.IsInstalled() {
			if err := yggSvc.WriteConfig(core.PrivKeyHex(node.PrivateKey())); err != nil {
				fmt.Println("\nFailed to write config:", err)
				os.Exit(1)
			}
		}

		if err := yggSvc.Start(); err != nil {
			fmt.Println("\nFailed to start TUN:", err)
			fmt.Println("Hint: Run PowerShell as Administrator")
			os.Exit(1)
		}

		fmt.Println(" ready.")
		fmt.Println("TUN active — browser can reach Yggdrasil addresses directly.")

	} else {
		// non-TUN mode: embedded library handles routing
		fmt.Print("Connecting to mesh")
		node.BootstrapPeers()
		time.Sleep(3 * time.Second)
		fmt.Println(" done.")
	}

	// ── DHT ─────────────────────────────────────────────────────────────────
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

	d.BootstrapDHT()

	if *peer != "" {
		if err := d.PingPeer(*peer); err != nil {
			fmt.Println("Could not reach peer:", err)
		}
	}

	// ── name + announce ──────────────────────────────────────────────────────
	nodeName := *name
	if nodeName == "" {
		nodeName = "node-" + node.PublicKey()[:8]
	}

	d.StartAPI(nodeName, node.Address(), node.PublicKey())

	var serviceList []string
	if *services != "" {
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
	}

	reannouncer := dht.NewReannouncer(d, record)
	reannouncer.Start()

	// ── ready ────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Name:    %s\n", nodeName)
	fmt.Printf("  Address: %s\n", node.Address())
	if *tun {
		fmt.Printf("  Browser: http://[%s]\n", node.Address())
	}
	if len(serviceList) > 0 {
		fmt.Printf("  Services: %v\n", serviceList)
	}
	fmt.Printf("  Find me: meshnet lookup %s\n", nodeName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  Running. Press Ctrl+C to stop.")
	fmt.Println()

	// ── shutdown ─────────────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down...")
	reannouncer.Stop()
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
		fmt.Println(`Look up a name on the mesh

USAGE:
  meshnet lookup <name> [flags]

EXAMPLES:
  meshnet lookup alice
  meshnet lookup myserver`)
	}
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Println("Usage: meshnet lookup <name>")
		os.Exit(1)
	}

	name := fs.Arg(0)

	if !dht.IsNodeRunning() {
		fmt.Println("No MeshNet node is running. Start one with: meshnet start")
		os.Exit(1)
	}

	// check contacts first — instant, no DHT query needed
	contacts, err := pairing.LoadContacts()
	if err == nil {
		if c := contacts.FindByName(name); c != nil {
			fmt.Printf("\nFound in contacts: %s\n", name)
			fmt.Printf("  Address: %s\n", c.Address)
			fmt.Printf("  Paired:  %s ago\n", time.Since(c.PairedAt).Round(time.Minute))
			return
		}
	}

	// not in contacts — query DHT
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
	fmt.Printf("  Key:      %s...\n", record.PublicKey[:16])
	if len(record.Services) > 0 {
		fmt.Printf("  Services: %v\n", record.Services)
	}
	fmt.Printf("  Expires:  %s\n", time.Until(time.Unix(record.Expires, 0)).Round(time.Minute))
}

// ── status ───────────────────────────────────────────────────────────────────

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
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

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Name:    %v\n", status["name"])
	fmt.Printf("  Address: %v\n", status["address"])
	fmt.Printf("  Key:     %v...\n", str16(status["public_key"]))
	fmt.Printf("  Peers:   %v\n", status["peers"])
	fmt.Printf("  Records: %v\n", status["records"])
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
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
		fmt.Println("Use 'meshnet peer add <address>' to connect to known nodes.")
		return
	}

	fmt.Printf("\nDHT Peers (%d)\n", len(peers))
	fmt.Println("────────────────────────────────────────────────────")
	for _, p := range peers {
		status := "✓"
		latency := p.Latency.Round(time.Millisecond).String()
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
		if !dht.IsNodeRunning() {
			fmt.Println("No MeshNet node is running.")
			os.Exit(1)
		}
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Post(
			fmt.Sprintf("http://127.0.0.1:%d/peer?addr=%s", dht.APIPort, args[1]),
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
		fmt.Println("Peer added.")

	case "list":
		data, err := os.ReadFile("peers.json")
		if err != nil {
			fmt.Println("No saved peers.")
			return
		}
		fmt.Println(string(data))

	case "clear":
		if err := os.Remove("peers.json"); err != nil {
			fmt.Println("No peers file to clear.")
			return
		}
		fmt.Println("Cleared all saved peers.")

	default:
		fmt.Printf("Unknown subcommand: %s\n", args[0])
		fmt.Println("Use: add, list, or clear")
		os.Exit(1)
	}
}

// ── pair ─────────────────────────────────────────────────────────────────────

func cmdPair(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println(`Pair with another MeshNet device

USAGE:
  meshnet pair              Generate a pairing code
  meshnet pair MESH-XXXX    Join using a code from another device

EXAMPLES:
  meshnet pair
  meshnet pair MESH-4729`)
	}
	fs.Parse(args)

	if !dht.IsNodeRunning() {
		fmt.Println("No MeshNet node is running. Start one with: meshnet start")
		os.Exit(1)
	}

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

	privKey, err := loadPrivateKey()
	if err != nil {
		fmt.Println("Failed to load private key:", err)
		os.Exit(1)
	}

	contacts, err := pairing.LoadContacts()
	if err != nil {
		fmt.Println("Failed to load contacts:", err)
		os.Exit(1)
	}

	d, cleanup, err := startPairingDHT(nodeAddress, privKey)
	if err != nil {
		fmt.Println("Failed to connect to DHT:", err)
		os.Exit(1)
	}
	defer cleanup()

	var contact *pairing.Contact

	if fs.NArg() == 0 {
		contact, err = pairing.Initiate(d, nodeName, nodeAddress, privKey)
	} else {
		contact, err = pairing.Join(d, nodeName, nodeAddress, privKey, fs.Arg(0))
	}

	if err != nil {
		fmt.Println("Pairing failed:", err)
		os.Exit(1)
	}

	contacts.Add(*contact)
	if err := contacts.Save(); err != nil {
		fmt.Println("Warning: could not save contact:", err)
	} else {
		fmt.Printf("Saved %s to contacts.\n", contact.Name)
	}
}

// ── contacts ─────────────────────────────────────────────────────────────────

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
	fmt.Println("──────────────────────────────────────────────────────")
	for _, c := range all {
		fmt.Printf("  %-20s  %s\n", c.Name, c.Address)
		fmt.Printf("  %-20s  paired %s ago\n", "",
			time.Since(c.PairedAt).Round(time.Minute))
	}
	fmt.Println("──────────────────────────────────────────────────────")
}

// ── helpers ───────────────────────────────────────────────────────────────────

// loadPrivateKey reads the ed25519 private key from identity.json
func loadPrivateKey() (ed25519.PrivateKey, error) {
	identityPath := os.Getenv("IDENTITY")
	if identityPath == "" {
		identityPath = "identity.json"
	}

	data, err := os.ReadFile(identityPath)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", identityPath, err)
	}

	var identity struct {
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("could not parse identity file: %w", err)
	}

	keyBytes, err := hex.DecodeString(identity.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key encoding: %w", err)
	}

	return ed25519.PrivateKey(keyBytes), nil
}

// startPairingDHT creates a temporary DHT instance for pairing operations
func startPairingDHT(address string, privKey ed25519.PrivateKey) (*dht.DHT, func(), error) {
	pubKey := privKey.Public().(ed25519.PublicKey)
	selfID, err := dht.NodeIDFromBytes(pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create node ID: %w", err)
	}

	d := dht.New(address, selfID, 9010)
	if err := d.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start pairing DHT: %w", err)
	}

	// bootstrap from running node's known peers
	client := &http.Client{Timeout: 5 * time.Second}
	if resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/peers", dht.APIPort)); err == nil {
		defer resp.Body.Close()
		var peers []dht.PeerInfo
		if json.NewDecoder(resp.Body).Decode(&peers) == nil {
			for _, p := range peers {
				d.PingPeer(fmt.Sprintf("[%s]:%d", p.Addr, p.Port))
			}
		}
	}

	return d, func() { d.Stop() }, nil
}

// str16 safely trims an interface{} string to 16 chars for display
func str16(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 16 {
		return s[:16]
	}
	return s
}
