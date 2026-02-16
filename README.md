# MeshNet

A decentralized peer-to-peer network layer that adds human-readable names and private groups to the Yggdrasil mesh network. No central servers. No company in the middle. Works through NAT.

## What It Does

MeshNet lets you create private networks between devices using names instead of cryptographic addresses:

- **Human-readable names**: Register as `alice` instead of `200:a1b2:c3d4:...`
- **Device pairing**: QR code or short code pairing between devices
- **Private groups**: Your network is invisible to non-members
- **NAT traversal**: Works behind routers without port forwarding
- **Permanent identity**: Your identity is derived from your cryptographic key, not assigned by a server
- **Fully decentralized**: No central servers, no company dependency

## Built On Yggdrasil

MeshNet is built as a layer on top of [Yggdrasil Network](https://yggdrasil-network.github.io/), an encrypted IPv6 overlay network. Yggdrasil provides:

- End-to-end encrypted mesh routing
- Permanent cryptographic identities
- NAT traversal via mesh connectivity
- Decentralized network topology

MeshNet adds human-readable names, device pairing, and private group functionality to the Yggdrasil mesh. We use Yggdrasil as a library and never modify its source code - all improvements made by the Yggdrasil team automatically benefit MeshNet users.

**Credit**: The Yggdrasil Network project and its contributors have built the foundation that makes MeshNet possible.

## Architecture

MeshNet is a three-layer system:

1. **Yggdrasil (Layer 1)**: Provides encrypted mesh routing and permanent IPv6 addresses
2. **Kademlia DHT (Layer 2)**: Distributed name registry running over the mesh
3. **Application (Layer 3)**: CLI and pairing system

## Current Status

**Under Active Development**: Core DHT and pairing systems are being built and tested. Not ready for production use.

### What's Implemented

- Permanent cryptographic identity generation
- Yggdrasil mesh integration with TUN support
- Kademlia DHT implementation (under testing)
- Name registration and lookup (under testing)
- Peer discovery and persistence (under testing)
- Record re-announcement (under testing)
- Local HTTP API for daemon/CLI separation
- Device pairing system (partial implementation)

### Current Limitations

- No bootstrap nodes available (requires manual peer addition)
- DHT functionality requires further testing and validation
- Pairing system incomplete
- Name conflict resolution not implemented
- Security features not yet implemented

## Quick Start

### Prerequisites

- Go 1.25.4 or later
- Yggdrasil v0.5.12 binary (for TUN mode)
- Administrator/root access (for TUN adapter)

### Build
```bash
git clone https://github.com/yourusername/meshnet
cd meshnet

# Download Yggdrasil binary from:
# https://github.com/yggdrasil-network/yggdrasil-go/releases/tag/v0.5.12
# Extract yggdrasil.exe and wintun.dll to bin/

go build -o meshnet.exe .
```

### Basic Usage
```bash
# Start a node and register a name
meshnet.exe start --name alice --tun

# Look up someone on the network
meshnet.exe lookup bob

# Check node status
meshnet.exe status

# List known peers
meshnet.exe peers

# Add a peer manually
meshnet.exe peer add "[200:x:x:x:x:x:x:x]:9002"
```

## Technical Details

### Identity

Each node generates a permanent ed25519 keypair on first run. This keypair:
- Derives your Yggdrasil IPv6 address
- Signs your DHT records
- Proves ownership of your registered name

Identity is stored in `identity.json`. Never commit this file.

### Name Registry

Names are stored in a Kademlia DHT with:
- 1-hour TTL with automatic re-announcement
- Cryptographic signature verification
- First-registered ownership model
- Optional group scoping for private networks

### Network Structure
```
Your Device (behind NAT)
    ↓
Yggdrasil Peer (public node)
    ↓
Global Yggdrasil Mesh
    ↓
Other Devices (anywhere on mesh)
```

No central coordination required. Packets route through the mesh using Yggdrasil's encrypted overlay network.

## Roadmap

### Immediate Next Steps

- Complete and test device pairing system
- Establish at least one bootstrap node
- Complete DHT testing and validation
- Implement security features

### Planned Features

- DNS resolver for `.mesh` domains
- Windows Service / background daemon
- System tray application
- Mobile apps

## Known Issues

- No bootstrap nodes available
- TUN adapter occasionally requires retry on startup
- Name conflict resolution not implemented
- Private keys stored in plaintext
- No local API authentication
- No DHT rate limiting
- No peer authentication

See `dev-docs/meshnet-problems.md` for complete documentation.

## Security Status

**Not production-ready.** Known security issues include:

- Private keys stored in plaintext
- No local API authentication
- No DHT STORE rate limiting
- No peer authentication during handshake

These must be addressed before any production use.

## Documentation

- `dev-docs/meshnet-idea.md` - Vision and architecture decisions
- `dev-docs/meshnet-build.md` - Build history and technical details
- `dev-docs/meshnet-context.md` - Developer onboarding guide
- `dev-docs/meshnet-problems.md` - Known issues and limitations

## Related Projects

- [Yggdrasil Network](https://yggdrasil-network.github.io/) - The mesh network layer MeshNet is built on
- [Yggdrasil GitHub](https://github.com/yggdrasil-network/yggdrasil-go) - Yggdrasil source code


## Philosophy

MeshNet is designed on three principles:

1. **True decentralization**: No central servers anywhere in the stack
2. **User sovereignty**: Your identity and network are yours, permanently
3. **Pragmatic design**: Build on proven technology rather than reinventing wheels

The goal is not to replace the internet, but to provide a private network layer that anyone can use without trusting a company.

## License

MIT License - see LICENSE file for details.

MeshNet is built on [Yggdrasil Network](https://github.com/yggdrasil-network/yggdrasil-go), which is licensed under LGPL v3.

---

**Status**: Early Development - Not Production Ready
