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

// yggAdminAddr is where the installed Yggdrasil service admin socket listens
const yggAdminAddr = "localhost:9001"

// yggSubprocessAdminAddr is where our subprocess admin socket listens
const yggSubprocessAdminAddr = "localhost:9091"

// yggLogFile is where subprocess output goes — keeps our CLI output clean
const yggLogFile = "yggdrasil.log"

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
	logFile *os.File
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

	return nil
}

// IsInstalled checks if the Yggdrasil Windows Service is installed
func (s *YggService) IsInstalled() bool {
	cmd := exec.Command("sc", "query", "Yggdrasil")
	err := cmd.Run()
	return err == nil
}

// isSubprocessRunning checks if our subprocess admin socket is reachable
func (s *YggService) isSubprocessRunning() bool {
	conn, err := net.DialTimeout("tcp", yggSubprocessAdminAddr, 300*time.Millisecond)
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
		fmt.Println("Using installed Yggdrasil service.")
		s.addPeersViaAdmin()
		return nil
	}

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
	// prevents "file already exists" on WinTun driver
	exec.Command("netsh", "interface", "delete", "interface", "Yggdrasil").Run()
	time.Sleep(500 * time.Millisecond)

	s.cmd = exec.Command(absPath, "-useconffile", absCfg, "-logto", "stdout")

	// redirect subprocess output to log file — keeps CLI output clean
	logFile, err := os.OpenFile(yggLogFile,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// can't open log file — fall back to stdout
		s.cmd.Stdout = os.Stdout
		s.cmd.Stderr = os.Stderr
	} else {
		s.logFile = logFile
		s.cmd.Stdout = logFile
		s.cmd.Stderr = logFile
	}

	if err := s.cmd.Start(); err != nil {
		if s.logFile != nil {
			s.logFile.Close()
		}
		return fmt.Errorf("failed to start yggdrasil: %w", err)
	}

	// wait for subprocess admin socket on 9091
	// it comes up before TUN is fully initialized
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		fmt.Print(".")
		if s.isSubprocessRunning() {
			// admin socket up — TUN initialization takes a few more seconds
			time.Sleep(5 * time.Second)
			return nil
		}
	}

	s.cmd.Process.Kill()
	if s.logFile != nil {
		s.logFile.Close()
	}
	return fmt.Errorf("yggdrasil failed to start — check %s for details", yggLogFile)
}

// addPeersViaAdmin adds bootstrap peers to the running installed service
func (s *YggService) addPeersViaAdmin() {
	peers := []string{
		"tls://62.210.85.80:39575",
		"tls://51.15.204.214:54321",
		"tls://n.ygg.yt:443",
		"tls://ygg7.mk16.de:1338?key=000000086278b5f3ba1eb63acb5b7f6e406f04ce83990dee9c07f49011e375ae",
		"tls://syd.joel.net.au:8443",
		"tls://95.217.35.92:1337",
		"tls://37.205.14.171:993",
	}

	conn, err := net.DialTimeout("tcp", yggAdminAddr, 2*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	for _, peer := range peers {
		req := fmt.Sprintf(`{"request":"addPeer","uri":"%s"}`, peer)
		conn.Write([]byte(req + "\n"))
		time.Sleep(100 * time.Millisecond)
	}
}

// GetAddress queries the subprocess admin socket for our mesh address
func (s *YggService) GetAddress() (string, error) {
	conn, err := net.DialTimeout("tcp", yggSubprocessAdminAddr, 3*time.Second)
	if err != nil {
		// try installed service socket as fallback
		conn, err = net.DialTimeout("tcp", yggAdminAddr, 3*time.Second)
		if err != nil {
			return "", fmt.Errorf("cannot reach yggdrasil admin: %w", err)
		}
	}
	defer conn.Close()

	req := map[string]interface{}{"request": "getSelf"}
	data, _ := json.Marshal(req)
	conn.Write(data)
	conn.Write([]byte("\n"))

	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read admin response: %w", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return "", fmt.Errorf("failed to parse admin response: %w", err)
	}

	if status, ok := resp["response"].(map[string]interface{}); ok {
		if addr, ok := status["address"].(string); ok {
			return addr, nil
		}
	}

	return "", fmt.Errorf("address not found in admin response")
}

// Stop shuts down the Yggdrasil subprocess
func (s *YggService) Stop() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	s.cmd.Process.Kill()
	s.cmd.Wait()

	if s.logFile != nil {
		s.logFile.Close()
	}

	// clean up TUN adapter so next run starts fresh
	exec.Command("netsh", "interface", "delete", "interface", "Yggdrasil").Run()
}
