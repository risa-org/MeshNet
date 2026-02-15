package core

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// yggConfigFile is where we write the Yggdrasil config
const yggConfigFile = "yggdrasil-meshnet.conf"

// yggAdminAddr is where Yggdrasil's admin socket listens
const yggAdminAddr = "localhost:9001"
const yggSubprocessAdminAddr = "localhost:9091"

// yggConfig is the minimal Yggdrasil configuration we need
type yggConfig struct {
	PrivateKey  string   `json:"PrivateKey"`
	Peers       []string `json:"Peers"`
	IfName      string   `json:"IfName"`
	IfMTU       int      `json:"IfMTU"`
	AdminListen string   `json:"AdminListen"`
}

// YggService manages Yggdrasil as a subprocess
// handles config generation, start, stop, and status
type YggService struct {
	cmd     *exec.Cmd
	binPath string
	cfgPath string
}

// NewYggService creates a new YggService
// binPath is the path to yggdrasil.exe
func NewYggService(binPath string) *YggService {
	return &YggService{
		binPath: binPath,
		cfgPath: yggConfigFile,
	}
}

// WriteConfig generates a Yggdrasil config file from our identity
// same private key = same Yggdrasil address = one unified identity
func (s *YggService) WriteConfig(privKeyHex string) error {
	cfg := yggConfig{
		PrivateKey: privKeyHex,
		Peers: []string{
			"tls://62.210.85.80:39575",  // france
			"tls://51.15.204.214:54321", // france
			"tls://n.ygg.yt:443",        // germany
			"tls://ygg7.mk16.de:1338?key=000000086278b5f3ba1eb63acb5b7f6e406f04ce83990dee9c07f49011e375ae", // austria
			"tls://syd.joel.net.au:8443", // australia
			"tls://95.217.35.92:1337",    // finland
			"tls://37.205.14.171:993",    // czechia
		},
		IfName:      "auto",
		IfMTU:       65535,
		AdminListen: yggSubprocessAdminAddr,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode yggdrasil config: %w", err)
	}

	if err := os.WriteFile(s.cfgPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write yggdrasil config: %w", err)
	}

	fmt.Printf("Yggdrasil config written to %s\n", s.cfgPath)
	return nil
}

func (s *YggService) IsInstalled() bool {
	cmd := exec.Command("sc", "query", "Yggdrasil")
	err := cmd.Run()
	return err == nil
}

// IsRunning checks if the Yggdrasil admin socket is reachable
// if it is, Yggdrasil is already running
func (s *YggService) IsRunning() bool {
	conn, err := net.DialTimeout("tcp", yggAdminAddr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Start launches yggdrasil.exe with our config
// requires Administrator privileges on Windows for TUN creation
func (s *YggService) Start() error {
	if s.IsInstalled() {
		fmt.Println("Installed Yggdrasil service detected — using existing TUN adapter")
		// add our peers to the running service via admin socket
		s.addPeersViaAdmin()
		return nil
	}

	// not installed — start subprocess
	absPath, err := filepath.Abs(s.binPath)
	if err != nil {
		return fmt.Errorf("invalid binary path: %w", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("yggdrasil binary not found at %s", absPath)
	}

	absCfg, err := filepath.Abs(s.cfgPath)
	if err != nil {
		return fmt.Errorf("invalid config path: %w", err)
	}

	// clean up leftover adapter from previous run
	exec.Command("netsh", "interface", "delete", "interface", "Yggdrasil").Run()
	time.Sleep(500 * time.Millisecond)

	s.cmd = exec.Command(absPath, "-useconffile", absCfg, "-logto", "stdout")
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start yggdrasil: %w", err)
	}

	fmt.Printf("Yggdrasil started (pid %d)\n", s.cmd.Process.Pid)
	fmt.Print("Waiting for TUN interface")
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		if s.IsRunning() {
			// admin socket up — wait extra for TUN to finish setup
			fmt.Print(" (TUN initializing)")
			time.Sleep(4 * time.Second)
			fmt.Println(" ready.")
			return nil
		}
		fmt.Print(".")
	}

	s.cmd.Process.Kill()
	return fmt.Errorf("yggdrasil failed to start within 15 seconds")
}

// addPeersViaAdmin adds bootstrap peers to the running Yggdrasil service
// via its admin socket — no config file editing needed
func (s *YggService) addPeersViaAdmin() {
	peers := []string{
		"tls://62.210.85.80:39575",  // france
		"tls://51.15.204.214:54321", // france
		"tls://n.ygg.yt:443",        // germany
		"tls://ygg7.mk16.de:1338?key=000000086278b5f3ba1eb63acb5b7f6e406f04ce83990dee9c07f49011e375ae", // austria
		"tls://syd.joel.net.au:8443", // australia
		"tls://95.217.35.92:1337",    // finland
		"tls://37.205.14.171:993",    // czechia
	}

	conn, err := net.DialTimeout("tcp", "localhost:9001", 2*time.Second)
	if err != nil {
		fmt.Println("Warning: could not reach Yggdrasil admin socket:", err)
		return
	}
	defer conn.Close()

	for _, peer := range peers {
		req := fmt.Sprintf(`{"request":"addPeer","uri":"%s"}`, peer)
		conn.Write([]byte(req + "\n"))
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("Peers added to Yggdrasil service")
}

// GetAddress queries the Yggdrasil admin socket for our address
// returns the 200: address assigned to the TUN interface
func (s *YggService) GetAddress() (string, error) {
	conn, err := net.DialTimeout("tcp", yggAdminAddr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("cannot reach yggdrasil admin: %w", err)
	}
	defer conn.Close()

	// send getSelf request
	req := map[string]interface{}{
		"request": "getSelf",
	}
	data, _ := json.Marshal(req)
	conn.Write(data)
	conn.Write([]byte("\n"))

	// read response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read admin response: %w", err)
	}

	// parse response
	var resp map[string]interface{}
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return "", fmt.Errorf("failed to parse admin response: %w", err)
	}

	// extract address
	if status, ok := resp["response"].(map[string]interface{}); ok {
		if addr, ok := status["address"].(string); ok {
			return addr, nil
		}
	}

	return "", fmt.Errorf("address not found in admin response")
}

// Stop shuts down the Yggdrasil subprocess we started
// if we didn't start it (it was already running) we leave it alone
func (s *YggService) Stop() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	fmt.Println("Stopping Yggdrasil subprocess...")
	s.cmd.Process.Kill()
	s.cmd.Wait()

	// remove the TUN adapter so next run starts clean
	// this prevents the "file already exists" error on next startup
	cleanup := exec.Command("netsh", "interface", "delete", "interface", "Yggdrasil")
	cleanup.Run()
	fmt.Println("Yggdrasil stopped.")
}
