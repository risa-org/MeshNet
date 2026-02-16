# MeshNet — Build Document
> What is built, how it works, what is left, and why every decision was made.

*Companion to meshnet-idea.md — read that first for context.*
*Document version 3.0 — post TUN integration*

---

## Current State

Phase 3 is functionally complete. The node generates a permanent cryptographic identity, joins the global Yggdrasil mesh, runs a full Kademlia DHT over that mesh, registers human-readable names with cryptographic signatures, finds other nodes by name, persists peers across restarts, re-announces records before they expire, and exposes a clean CLI and local HTTP API. Two nodes on separate machines can find each other by name with no central server involved anywhere.

Phases 1, 2, and 3 are done.

---

## What Is Built

### core/identity.go
Manages the node's permanent cryptographic identity. On first run generates a fresh ed25519 key pair, encodes both keys as hex, saves to `identity.json` with 0600 permissions. Every subsequent run loads from that file. Same keys = same Yggdrasil address = permanent identity forever.

The identity file path is configurable via `IDENTITY` environment variable — used during development to run multiple nodes on the same machine with different identities.

```
identity.json — NEVER commit. Contains private key. In .gitignore.
```

### core/cert.go
Wraps the ed25519 key pair into a TLS certificate. Yggdrasil v0.5.x accepts identity via `*tls.Certificate` not raw keys. Self-signed X.509 cert valid 100 years. Content of certificate fields doesn't matter — only the embedded keys do.

### core/node.go
The only file that knows Yggdrasil exists. Exposes clean API: `Start()`, `Stop()`, `Address()`, `PublicKey()`, `PrivateKey()`, `AddPeer()`, `Bootstrap()`. Everything else calls these functions.

`PrivateKey()` was added in Phase 3 so the DHT layer can sign records with the node's ed25519 private key — same identity used for both mesh routing and DHT record ownership.

Bootstrap connects to five community TLS peers across UK, DE, FR, AU, PL in parallel goroutines. Only one needs to be alive to join the mesh.

### dht/routing.go
Kademlia routing table. Every node has a 32-byte NodeID derived from its ed25519 public key. The routing table organizes known nodes into 256 buckets by XOR distance from our own ID. Bucket 0 = nodes farthest from us. Bucket 255 = nodes closest.

Key functions:
- `NodeID.XOR(other)` — computes distance between two IDs
- `NodeID.Less(other, target)` — compares which of two nodes is closer to a target
- `NodeID.bucketIndex(other)` — which bucket a node belongs in
- `RoutingTable.Add(contact)` — adds node, updates if existing, drops if bucket full
- `RoutingTable.Closest(target, k)` — returns K closest known nodes to a target ID
- `RoutingTable.All()` — returns all contacts, used for peer persistence

### dht/store.go
In-memory record database with TTL and signature verification. Records expire after 1 hour. Background cleanup goroutine runs every minute removing expired records.

Every `Put()` call verifies the record's cryptographic signature before storing. Rejects expired records. Prevents name squatting — once a name is registered by a key, only that key can update it.

The `SigningPayload()` function computes a SHA256 hash of all record fields except the signature itself. This exact payload is signed during creation and verified during storage. Critical: the Signature field must NOT be included in the payload or verification fails on existing records.

### dht/rpc.go
Wire protocol for node-to-node communication. Every message: `[1 byte type][4 bytes length][N bytes JSON body]`. Six message types: PING/PONG, FIND_NODE/FOUND_NODES, STORE, FIND_VALUE/FOUND_VALUE/NOT_FOUND.

All outgoing send functions (`SendPing`, `SendFindNode`, `SendStore`, `SendFindValue`) dial, send, and read response in one call — no persistent connections. Clean and simple.

### dht/lookup.go
Active side of the DHT. Three operations:

**LookupNode(target)** — iterative Kademlia node lookup. Seeds from routing table. Queries alpha=3 nodes concurrently per round. Collects closer nodes from responses. Converges when no progress. Returns K closest nodes found.

**LookupValue(name, groupKey)** — same iterative process but stops the moment any node returns the record. Checks local store first before querying network.

**Announce(record)** — stores record locally first, then finds K closest nodes and sends STORE to each concurrently. Reports how many nodes accepted the record.

### dht/register.go
Creates signed DHT records from a set of options. Takes name, address, services, group key, and private key. Builds the record, computes signing payload, signs with ed25519, attaches signature. The record is now ready for DHT storage and will pass verification on any node.

### dht/announce.go
Re-announcement loop. `Reannouncer` struct holds a record and re-announces it every 45 minutes — within the 1-hour TTL so records never expire from the DHT while the node is running. Clean Start/Stop lifecycle.

### dht/peers.go
Peer persistence. `SavePeers()` writes all routing table contacts to `peers.json` on shutdown. `LoadPeers()` reads them on startup and pings each concurrently — only live peers get added back. `BootstrapDHT()` tries saved peers first, then well-known bootstrap nodes, reports isolated mode if none reachable. `PingAllPeers()` pings all known peers and returns latency and alive status for the CLI.

### dht/bootstrap_peers.go
Well-known MeshNet bootstrap nodes. Currently empty placeholder — no community nodes exist yet. Same pattern as Yggdrasil's bootstrap peers. When the network has participants this list grows via community PRs. A node that goes offline is simply skipped.

### dht/dht.go
Top-level DHT coordinator. Owns the TCP listener, routing table, store, and API server. Accept loop runs in background goroutine. Each incoming connection handled in its own goroutine.

Incoming message dispatch: PING → handlePing (add sender to routing table, return our info), FIND_NODE → handleFindNode (return K closest contacts), STORE → handleStore (verify and store record), FIND_VALUE → handleFindValue (return record if found, else closest nodes).

Critical fix applied: `handlePing` returns `d.port` not the constant `DHTPort` — otherwise nodes behind non-default ports report the wrong address and get removed from routing tables.

`PingPeer()` uses the dialed address not the reported address for storing contacts — ensures reachability when nodes are on the same machine.

### dht/api.go
Local HTTP API on `127.0.0.1:9099`. Only binds to localhost — never exposed to mesh or internet. Endpoints: `GET /status`, `GET /lookup?name=&group=`, `GET /peers`, `POST /peer?addr=`. Allows CLI commands to talk to a running node without starting their own DHT instance.

`IsNodeRunning()` checks for a running node by hitting `/status` with a 500ms timeout. CLI commands check this before doing anything.

### cli/cli.go
Full CLI with five commands: `start`, `lookup`, `status`, `peers`, `peer`. All use Go's standard `flag` package — no external dependencies.

`start` — full node startup with flags: `--name`, `--port`, `--identity`, `--peer`, `--services`. Starts node, bootstraps DHT, announces record, starts reannouncer, starts local API, blocks until Ctrl+C, saves peers on shutdown.

`lookup`, `status`, `peers`, `peer add` — talk to running node via local API. No port conflicts. No second DHT instance. If no node is running, print helpful error.

### main.go
Three lines. Calls `cli.Run()`. That's it.

---

## The Full Startup Sequence (meshnet start --name alice)

```
1. Parse CLI flags
2. Set IDENTITY env var
3. core.NewNode().Start()
   a. loadOrCreateIdentity() → ed25519 keypair
   b. generateSelfSignedCert() → TLS cert
   c. yggcore.New(cert, logger) → Yggdrasil starts, mesh routing active
   d. admin.New() → admin socket
   e. store address
4. node.Bootstrap() → 5 peer connections in goroutines
5. time.Sleep(3s) → let peers connect
6. dht.New(address, selfID, port)
7. d.Start()
   a. net.Listen("[::]:<port>") → TCP listener open
   b. go acceptLoop() → accepting connections
   c. store.Start() → cleanup goroutine running
8. d.BootstrapDHT()
   a. LoadPeers() → ping saved peers, add live ones to routing table
   b. if still empty, try bootstrap nodes
9. d.PingPeer(--peer flag) → manual peer if specified
10. dht.CreateRecord() → sign record with private key
11. time.Sleep(1s) → let routing table settle
12. d.Announce(record) → store locally + distribute to K closest nodes
13. d.StartAPI(name, address, pubkey) → HTTP API on :9099
14. dht.NewReannouncer(d, record).Start() → 45min re-announce loop
15. Block on os.Signal
16. On Ctrl+C:
    a. reannouncer.Stop()
    b. d.SavePeers() → write peers.json
    c. d.Stop() → close listener, stop store, wait for goroutines
    d. node.Stop() → shutdown Yggdrasil
```

---

## The Yggdrasil/OS Integration Problem

**This is the most important unresolved issue in the current version.**

MeshNet embeds Yggdrasil as a library. This means:
- Mesh routing works ✓
- DHT works over the mesh ✓
- OS-level TUN adapter does NOT exist ✗
- Browser cannot reach Yggdrasil addresses ✗
- Other OS applications cannot use the mesh ✗

The TUN adapter requires a kernel-level network interface driver. The standalone Yggdrasil installer handles this with a TAP/TUN driver and elevated system permissions. Our embedded library does not do this automatically.

**Current workaround:** Install standalone Yggdrasil alongside MeshNet. Two separate processes, two separate keypairs, two separate addresses. The installed Yggdrasil handles OS routing. MeshNet handles DHT. They coexist but have different identities — which means the address in DHT records is the MeshNet address, not the OS-routable Yggdrasil address.

**The correct fix (Phase 6):** MeshNet manages everything. Reads or shares the installed Yggdrasil keypair, or installs its own TUN driver, or ships the Yggdrasil driver as part of the installer. One keypair, one address, one process. The installed standalone Yggdrasil is not needed.

**Why deferred:** TUN driver installation is complex Windows-specific work that requires an installer, elevated privileges, and kernel driver signing. It is right to solve this properly in Phase 6 rather than half-solve it now.

---

## Bugs Fixed During Phase 3

**SigningPayload included Signature field** — caused all signature verifications to fail silently. `store.Put()` rejected every record. `Announce` stored 0 copies. Fixed by removing Signature from the signing payload struct.

**handlePing returned constant DHTPort** — nodes behind non-default ports reported wrong port in pong response. Dialing them using reported address dialed the wrong port. `LookupNode` got connection errors, called `Remove()`, deleted the only known peer. Fixed by returning `d.port`.

**PingPeer stored reported address not dial address** — on same-machine testing, the reported Yggdrasil address was unreachable locally. Stored dial address (from the addr string we actually connected to) instead.

**LookupNode removed nodes too aggressively** — any error including timeout triggered `Remove()`. Changed to log the error and skip without removing. Slow nodes shouldn't be permanently expelled.

**`break` in select didn't break outer loop** — in `LookupValue` the timeout case used `break` which only broke the select. Changed to `return nil, nil` to actually exit the function.

**Windows cannot bind to Yggdrasil IPv6 address directly** — `net.Listen` on the specific `200:` address fails on Windows. Fixed by binding to `[::]` (all IPv6 interfaces). Connections through the Yggdrasil mesh still arrive.

---

## Project File Structure (Current)

```
meshnet/
├── .gitignore
├── go.mod
├── go.sum
├── main.go                  — 3 lines, calls cli.Run()
├── meshnet.exe              — compiled binary
├── identity.json            — YOUR PRIVATE KEY. Never commit.
├── identity2.json           — second identity for local testing
├── peers.json               — saved DHT peers, auto-managed
│
├── core/
│   ├── identity.go          — ed25519 keypair persistence
│   ├── cert.go              — TLS cert from keypair
│   └── node.go              — Yggdrasil wrapper
│
├── dht/
│   ├── dht.go               — coordinator, listener, message handlers
│   ├── routing.go           — Kademlia routing table, XOR distance
│   ├── store.go             — signed records with TTL
│   ├── rpc.go               — wire protocol, send functions
│   ├── lookup.go            — iterative lookup, announce
│   ├── register.go          — signed record creation
│   ├── announce.go          — periodic re-announcement
│   ├── peers.go             — peer persistence and bootstrap
│   ├── bootstrap_peers.go   — well-known community nodes (empty)
│   └── api.go               — local HTTP API on :9099
│
└── cli/
    └── cli.go               — full CLI: start, lookup, status, peers, peer
```

---

## CLI Reference

```
meshnet start                          Start node with default name
meshnet start --name alice             Start with specific name
meshnet start --name alice --port 9002 Different DHT port
meshnet start --identity id2.json      Different identity file
meshnet start --peer "[::1]:9002"      Manual bootstrap peer
meshnet start --services ssh:22,http:80  Announce services

meshnet lookup alice                   Find alice on the mesh
meshnet lookup alice --group <key>     Find alice in private group

meshnet status                         Show running node status
meshnet peers                          List known peers with latency
meshnet peer add "[200:...]:9002"      Add peer to running node
meshnet peer list                      Show saved peers file
meshnet peer clear                     Clear all saved peers
```

All commands except `start` require a node to already be running. They talk to the local API on port 9099.

---

## What Is Left To Build

### Phase 4 — Pairing System
Two devices establish mutual trust without manual address exchange. User sees a short code like `MESH-4729`. Other device enters it. Both nodes exchange public keys, add each other as trusted peers, join a shared private group. No addresses. No config. Works for non-technical people.

What to build:
- Short code generation (6-8 chars, human typeable)
- QR code generation from pairing code
- Temporary rendezvous mechanism via DHT public records
- Mutual public key exchange during handshake
- Shared group key derivation from paired keys
- Local contact list (name + address + group membership)
- Pairing expiry (codes valid for 5-10 minutes only)

### Phase 5 — DNS Resolver
Makes `.mesh` names work in any browser without configuration. A local DNS stub resolver listens on `127.0.0.1:53`. Intercepts `.mesh` queries, resolves via DHT, returns Yggdrasil address. All other queries forwarded to normal DNS. OS configured to use local resolver.

What to build:
- DNS server using `miekg/dns` library (~100 lines)
- `.mesh` query interception and DHT lookup
- Response caching with TTL matching DHT record expiry
- OS DNS configuration (Windows: registry, Linux: systemd-resolved, Mac: /etc/resolver)
- Fallback if DHT lookup fails

### Phase 6 — OS Integration and Single Identity
Unify the Yggdrasil and MeshNet identities. One keypair. One address. One process. Browser routing and DHT use the same identity.

What to build:
- Read/share identity with installed Yggdrasil OR
- MeshNet installs its own TUN driver and creates the interface
- Windows Service registration (start on boot, run silently)
- System tray icon with status, quit, open UI
- Installer that handles everything (keypair, TUN driver, service)

### Phase 7 — Mobile and Desktop UI
Real application UI hiding all complexity.

What to build:
- Desktop: system tray app, contact list, pairing flow
- Mobile: background service, simple UI, QR scanning
- Local HTTP API expansion for UI consumption
- Push-style notifications for new connections

---

## Technical Decisions — All Locked

**Go** — Yggdrasil is Go. Same language = direct function calls. No IPC. No translation.

**Yggdrasil as library** — one binary. Direct function calls. No subprocess management. No IPC overhead.

**Yggdrasil never modified** — dependency only. Update via go.mod. Never fork.

**Kademlia DHT** — proven algorithm. BitTorrent-scale validated. No blockchain overhead. Fast. Lightweight. Works on mobile.

**ed25519 for all signing** — same key used for Yggdrasil identity and DHT record ownership. Cryptographic proof of name ownership.

**Local HTTP API** — daemon/CLI separation. Running node owns the ports. CLI commands talk to it. No port conflicts. Standard Unix daemon pattern.

**No external CLI dependencies** — standard library `flag` package only. Binary stays lean.

**DHT port 9002** — avoids conflict with standalone Yggdrasil which uses 9001 for peer connections.

---

## The One Rule That Never Changes

Yggdrasil source code is never modified. It is a dependency. Everything is achievable from the outside through configuration and wrapping.