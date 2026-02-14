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
	peers := []string{
		"tls://uk.lhc.network:17002",
		"tls://de.lhc.network:17002",
		"tls://fr.lhc.network:17002",
		"tls://au.lhc.network:17002",
		"tls://pl.lhc.network:17002",
	}
	for _, peer := range peers {
		go func(p string) {
			err := n.AddPeer(p)
			if err != nil {
				fmt.Printf("Could not add peer %s : %v\n", p, err)
			} else {
				fmt.Printf("Peer Added %s\n", p)
			}
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
