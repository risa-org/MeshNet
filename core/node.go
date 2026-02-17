package core

import (
	"crypto/ed25519"
	"fmt"
	"net/url"
	"os"

	"github.com/gologme/log"
	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	yggcore "github.com/yggdrasil-network/yggdrasil-go/src/core"
)

type Node struct {
	core    *yggcore.Core
	admin   *admin.AdminSocket
	logger  *log.Logger
	address string
	privKey ed25519.PrivateKey
}

func NewNode() *Node {
	return &Node{}
}

func (n *Node) Start() error {
	n.logger = log.New(os.Stderr, "", 0)
	n.logger.EnableLevel("info")
	n.logger.EnableLevel("warn")
	n.logger.EnableLevel("error")

	pubKey, privKey, err := loadOrCreateIdentity()
	if err != nil {
		return fmt.Errorf("failed to load identity %w", err)
	}
	n.privKey = privKey

	cert, err := generateSelfSignedCert(pubKey, privKey)
	if err != nil {
		return fmt.Errorf("failed to generate certificate %w", err)
	}

	n.core, err = yggcore.New(cert, n.logger)
	if err != nil {
		return fmt.Errorf("failed to create yggdrasil node: %w", err)
	}

	n.admin, err = admin.New(n.core, n.logger)
	if err != nil {
		return fmt.Errorf("failed to create admin socket: %w", err)
	}

	n.address = n.core.Address().String()
	return nil
}

func (n *Node) AddPeer(peerURL string) error {
	u, err := url.Parse(peerURL)
	if err != nil {
		return fmt.Errorf("invalid peer URL %s: %w", peerURL, err)
	}
	return n.core.AddPeer(u, "")
}

func (n *Node) Bootstrap() {
	// peers are added externally only when NOT in TUN mode
	// in TUN mode the subprocess handles all routing
	// calling this when subprocess is running causes routing conflicts
}

func (n *Node) BootstrapPeers() {
	peers := []string{
		"tls://62.210.85.80:39575",
		"tls://51.15.204.214:54321",
		"tls://n.ygg.yt:443",
		"tls://ygg7.mk16.de:1338?key=000000086278b5f3ba1eb63acb5b7f6e406f04ce83990dee9c07f49011e375ae",
		"tls://syd.joel.net.au:8443",
		"tls://95.217.35.92:1337",
		"tls://37.205.14.171:993",
	}
	for _, peer := range peers {
		go func(p string) {
			u, err := url.Parse(p)
			if err != nil {
				return
			}
			if err := n.core.AddPeer(u, ""); err != nil {
				return
			}
			fmt.Println("Peer Added", p)
		}(peer)
	}
}
func (n *Node) Address() string {
	return n.address
}

func (n *Node) PublicKey() string {
	return fmt.Sprintf("%x", n.core.PublicKey())
}

func (n *Node) Stop() {
	if n.admin != nil {
		n.admin.Stop()
	}

	if n.core != nil {
		n.core.Stop()
	}

}

func (n *Node) PrivateKey() ed25519.PrivateKey {
	return n.privKey
}
