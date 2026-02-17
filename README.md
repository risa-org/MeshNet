# MeshNet

A decentralized naming and discovery layer for the [Yggdrasil](https://yggdrasil-network.github.io/) mesh network. Register human-readable names, discover peers, and build private groups — no central servers, no company in the middle.

```
meshnet start --name alice --tun
meshnet lookup bob
```

---

## What It Does

Yggdrasil gives every device a permanent cryptographic IPv6 address. The problem: those addresses look like `200:7ae5:22a0:c183:ec90:3b6e:3ad6:3e6a`. Nobody types that.

MeshNet adds a distributed name registry on top of Yggdrasil:

- **Human names** — register `alice`, look up `bob`, no DNS server involved
- **TUN integration** — browser, ping, curl all work with Yggdrasil addresses directly
- **Private groups** — scope your names to a group key, invisible to outsiders
- **Service discovery** — announce `ssh:22` or `http:80` alongside your name
- **Cryptographic ownership** — your name is signed with your keypair, nobody can steal it

MeshNet handles naming only. All routing, encryption, and NAT traversal is handled by Yggdrasil.

---

## How It Works

```
┌─────────────────────────────────┐
│  MeshNet (naming + discovery)   │  ← This project
│  Kademlia DHT over Yggdrasil    │
├─────────────────────────────────┤
│  Yggdrasil (mesh routing)       │  ← Dependency
│  Encrypted overlay, NAT traversal│
├─────────────────────────────────┤
│  Your internet connection       │
└─────────────────────────────────┘
```

Each node generates a permanent ed25519 keypair on first run. This keypair derives your Yggdrasil address and signs all your DHT records. Your name is cryptographically yours — first registered, permanently owned.

Records propagate through a [Kademlia DHT](https://en.wikipedia.org/wiki/Kademlia) running over Yggdrasil connections. No central server. No blockchain. Proven at BitTorrent scale.

---

## Setup

### Prerequisites

- Go 1.21 or later
- Windows (Linux/Mac support planned)
- Administrator access for TUN mode
- [Yggdrasil v0.5.12](https://github.com/yggdrasil-network/yggdrasil-go/releases/tag/v0.5.12) binary

### Install

```bash
git clone https://github.com/yourusername/meshnet
cd meshnet
```

Download from the [Yggdrasil v0.5.12 release](https://github.com/yggdrasil-network/yggdrasil-go/releases/tag/v0.5.12):
- `yggdrasil-0.5.12-x64.msi` — install it, or
- Extract `yggdrasil.exe` and `wintun.dll` into `bin/`

```bash
go build -o meshnet.exe .
```

### Run

```bash
# Start a node (TUN mode — browser access, requires admin)
.\meshnet.exe start --name alice --tun

# Start without TUN (no browser access, no admin needed)
.\meshnet.exe start --name alice
```

---

## Usage

```
meshnet start     Start the node and register your name
meshnet lookup    Find someone on the mesh by name
meshnet status    Show your running node's info
meshnet peers     List known DHT peers
meshnet peer      Add / list / clear peers
```

### Examples

```bash
# Register with a name and enable browser access
meshnet start --name alice --tun

# Register with services
meshnet start --name myserver --services ssh:22,http:80

# Look someone up
meshnet lookup bob
# Found: bob
#   Address:  200:b48d:469e:c7c7:...
#   Services: [ssh:22]

# Check your node
meshnet status

# Add a peer manually
meshnet peer add "[200:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx]:9001"
```

### TUN Mode

With `--tun`, MeshNet creates a network adapter so your OS routes Yggdrasil traffic natively. After starting with `--tun`:

```bash
# Ping a node directly
ping 200:7ae5:22a0:c183:ec90:3b6e:3ad6:3e6a

# Open in browser
http://[200:7ae5:22a0:c183:ec90:3b6e:3ad6:3e6a]
```

Requires running as Administrator on Windows.

---

## Architecture

### Files

```
meshnet/
├── main.go              Entry point
├── cli/cli.go           Command-line interface
├── core/
│   ├── identity.go      Keypair generation and persistence
│   ├── cert.go          TLS certificate for Yggdrasil
│   ├── node.go          Yggdrasil embedded node
│   └── yggservice.go    Yggdrasil subprocess (TUN mode)
├── dht/
│   ├── dht.go           DHT coordinator
│   ├── routing.go       Kademlia routing table
│   ├── store.go         Record storage and verification
│   ├── lookup.go        Iterative lookup and announce
│   ├── register.go      Signed record creation
│   ├── announce.go      Periodic re-announcement
│   ├── peers.go         Peer persistence and bootstrap
│   ├── rpc.go           Wire protocol
│   └── api.go           Local HTTP API
└── bin/
    ├── yggdrasil.exe    Not committed — download separately
    └── wintun.dll       Not committed — download separately
```

### Identity

Your identity lives in `identity.json`. This file:
- Is generated automatically on first run
- Contains your private key — **never commit it**
- Determines your permanent Yggdrasil address
- Is gitignored by default

If you have the Yggdrasil Windows Service installed, MeshNet reuses its keypair so you share one address across both.

### DHT Records

```json
{
  "name": "alice",
  "address": "200:7ae5:22a0:c183:ec90:3b6e:3ad6:3e6a",
  "public_key": "c28d6eaf9f3e09b7...",
  "services": ["http:80"],
  "group_key": "",
  "signature": "a1b2c3d4...",
  "expires": 1708123456
}
```

Records are signed with ed25519. Any node that receives a record verifies the signature before storing it. Ownership is first-come, permanent — same name from a different key gets rejected.

### TUN Architecture

In TUN mode MeshNet runs two Yggdrasil instances:

1. **Embedded library** — handles DHT communication
2. **Subprocess (`bin/yggdrasil.exe`)** — owns the TUN adapter for OS routing

Both use the same keypair so they share one address. The embedded library deliberately does **not** connect to peers in TUN mode — two instances announcing the same key causes routing conflicts on the mesh.

---

## Bootstrap Nodes

MeshNet DHT currently has no permanent bootstrap nodes. To connect two nodes:

```bash
# On machine B, after starting:
meshnet peer add "[200:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx]:9001"
```

Once connected, peers are saved to `peers.json` and restored on next start.

Community bootstrap nodes will be added as the network grows.

---

## Roadmap

- **Phase 4** — Device pairing via short codes (`MESH-XXXX`)
- **Phase 5** — DNS resolver so `alice.mesh` works in browser
- **Phase 6** — Windows Service, system tray, installer
- **Phase 7** — Mobile apps, QR code pairing

---

## Status

Early development. Core mesh routing and naming work. Not production-ready.

Known limitations:
- Windows only (Linux/Mac planned)
- No bootstrap nodes yet
- No DNS integration yet
- Private keys stored in plaintext
- No local API authentication

---

## License

MIT — see [LICENSE](LICENSE).

Built on [Yggdrasil Network](https://github.com/yggdrasil-network/yggdrasil-go) (LGPL v3).