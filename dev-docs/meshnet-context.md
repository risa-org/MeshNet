# MeshNet — Complete Context Document
> Everything needed to continue development in a new thread.
> Read this fully before writing any code.

---

## What MeshNet Is

A parallel internet layer running over Yggdrasil. Two modes on one DHT:

**Public layer** — anyone registers a human-readable name anchored to their public key. First registered owns it. No ICANN, no fees, cryptographic ownership proof.

**Private layer** — records tagged with a group key are invisible to non-members. Same DHT, same protocol, just a visibility field.

One identity. One keypair. One DHT. Browser access via TUN. Works through NAT. No central servers anywhere.

---

## Current State — What Works Right Now

```
meshnet start --name alice --tun    ← starts node, creates TUN, browser works
meshnet lookup alice                ← finds alice on mesh by name
meshnet status                      ← shows running node info
meshnet peers                       ← lists known DHT peers with latency
meshnet peer add [addr]:port        ← adds peer to running node
meshnet pair                        ← generate pairing code (PARTIALLY BUILT)
meshnet contacts                    ← list paired devices (PARTIALLY BUILT)
```

**Working fully:**
- Yggdrasil embedded mesh routing with real peer connections
- Kademlia DHT over the mesh — routing, storage, lookup, announce
- Cryptographic record signing with ed25519
- Peer persistence across restarts
- 45-minute re-announcement loop so records never expire
- Local HTTP API on 127.0.0.1:9099 for CLI↔daemon communication
- TUN adapter via Yggdrasil subprocess — browser can ping/reach Yggdrasil nodes
- Clean CLI with proper flag parsing
- Shutdown saves peers, cleans up TUN adapter

**Partially built (needs fixes before working):**
- Pairing — code written but has port conflict bug and needs security fixes

**Not built yet:**
- DNS resolver (`.mesh` names in browser)
- Windows Service / background daemon
- Installer
- Mobile/desktop UI

---

## Project File Structure

```
meshnet/
├── .gitignore
├── go.mod               (module meshnet, go 1.25.4)
├── go.sum
├── main.go              (3 lines — calls cli.Run())
├── identity.json        ← NEVER COMMIT. Private key.
├── peers.json           ← auto-managed, gitignored
├── yggdrasil-meshnet.conf ← generated at runtime, gitignored
├── contacts.json        ← pairing contacts, gitignored
│
├── bin/
│   ├── README.md        ← committed
│   ├── yggdrasil.exe    ← NOT committed (9MB)
│   └── wintun.dll       ← NOT committed (driver)
│
├── cli/
│   └── cli.go           ← full CLI
│
├── core/
│   ├── identity.go      ← keypair management
│   ├── cert.go          ← TLS cert generation
│   ├── node.go          ← Yggdrasil wrapper
│   └── yggservice.go    ← Yggdrasil subprocess manager
│
├── dht/
│   ├── dht.go           ← coordinator, TCP listener, message handlers
│   ├── routing.go       ← Kademlia routing table, XOR distance
│   ├── store.go         ← record storage, signature verification, TTL
│   ├── rpc.go           ← wire protocol
│   ├── lookup.go        ← iterative lookup, announce
│   ├── register.go      ← signed record creation
│   ├── announce.go      ← periodic re-announcement
│   ├── peers.go         ← peer persistence and bootstrap
│   ├── bootstrap_peers.go ← well-known nodes (currently empty)
│   └── api.go           ← local HTTP API on :9099
│
└── pairing/
    ├── pairing.go       ← pairing logic (partially complete)
    └── contacts.go      ← contacts persistence
```

---

## Key Technical Decisions — All Locked

**Go** — Yggdrasil is Go. Direct function calls, no translation layer. Never change this.

**Yggdrasil as embedded library** — handles mesh routing. Never modified. Update via go.mod only.

**Yggdrasil as subprocess for TUN** — v0.5.x library exposes no TUN API. Subprocess is correct. Binary at `bin/yggdrasil.exe`.

**Single identity per installation** — one keypair, permanent, tied to the machine. `--identity` flag exists for dev/testing only, hidden from production help text.

**Kademlia DHT** — XOR distance, 256 buckets, K=20, alpha=3. No blockchain. Proven at BitTorrent scale.

**Local HTTP API** — daemon owns the ports. CLI talks to it. Standard Unix daemon pattern. Port = DHT port + 98 (so 9001→9099, 9002→9100, no conflicts).

**ed25519 everywhere** — same key for Yggdrasil identity and DHT record signing. One identity, one address.

**No external CLI dependencies** — standard `flag` package only.

---

## Current Yggdrasil Bootstrap Peers (Working As Of Feb 2026)

These replaced the dead lhc.network peers:

```go
"tls://62.210.85.80:39575",    // France
"tls://51.15.204.214:54321",   // France
"tls://n.ygg.yt:443",          // Germany
"tls://ygg7.mk16.de:1338?key=000000086278b5f3ba1eb63acb5b7f6e406f04ce83990dee9c07f49011e375ae", // Austria
"tls://syd.joel.net.au:8443",  // Australia
"tls://95.217.35.92:1337",     // Finland
"tls://37.205.14.171:993",     // Czechia
```

Source: https://publicpeers.neilalexander.dev/ — check here for updates.

---

## The Identity System

`core/identity.go` loads identity in this priority order:

1. Try `C:\ProgramData\Yggdrasil\yggdrasil.conf` — if installed Yggdrasil service exists, reuse its keypair so MeshNet and OS share one address
2. Try `identity.json` (or path from `IDENTITY` env var)
3. Generate fresh keypair, save to `identity.json`

The Yggdrasil config parser uses line-by-line scanning (not JSON.Unmarshal) because Yggdrasil uses a custom format with `#` comments and unquoted keys.

---

## The TUN System

`core/yggservice.go` manages Yggdrasil subprocess:

1. `IsInstalled()` — checks `sc query Yggdrasil` for installed Windows Service
2. If installed service exists — use it, don't start subprocess
3. If not — clean up leftover adapter (`netsh interface delete interface Yggdrasil`)
4. Write `yggdrasil-meshnet.conf` with our private key and peer list
5. Launch `yggdrasil.exe -useconffile yggdrasil-meshnet.conf`
6. Wait for admin socket on `localhost:9091`
7. Wait 4s for TUN init + 5s for route stabilization

**Known issue:** First TUN creation attempt occasionally fails with "Cannot create a file when that file already exists" — WinTun driver timing. Yggdrasil always recovers automatically on retry. Cosmetic, not functional.

**The conf file contains the private key in plaintext.** This is a security issue — fix by deleting the file immediately after subprocess starts (subprocess already has it loaded in memory).

---

## DHT Record Schema

```go
type Record struct {
    Name      string   // human name e.g. "alice"
    Address   string   // Yggdrasil IPv6 address
    PublicKey string   // hex ed25519 public key
    Services  []string // e.g. ["ssh:22", "http:80"]
    GroupKey  string   // empty = public, filled = private group
    Signature string   // ed25519 signature of all other fields
    Expires   int64    // unix timestamp, 1 hour TTL
}
```

`SigningPayload()` hashes all fields EXCEPT Signature. Critical — including Signature in the hash breaks verification.

`store.Put()` verifies signature, rejects expired records, rejects name hijacking (same name different key).

---

## The Local API

Runs on `127.0.0.1:{DHT port + 98}`. Never exposed to mesh.

```
GET  /status                    → node name, address, pubkey, peer count, record count
GET  /lookup?name=&group=       → DHT lookup, returns Record JSON
GET  /peers                     → all peers with latency and alive status
POST /peer?addr=                → add peer, saves peers.json
POST /pair/initiate?name=       → create pairing record, returns {"code": "MESH-XXXX"}
GET  /pair/poll?code=           → check for pairing response, 202 if still waiting
POST /pair/join?code=&name=     → find initiator, announce response, returns ContactInfo
```

---

## What Needs To Be Fixed RIGHT NOW (Before Continuing)

### Fix 1 — API Port Conflict (CRITICAL for pairing)

**Problem:** Second node on `--port 9002` tries to bind API on 9099 which first node owns. Fails with bind error.

**Fix:** In `dht/api.go`, change API port from hardcoded 9099 to `d.port + 98`:

```go
func (d *DHT) APIPort() int {
    return d.port + 98
}
```

Update `StartAPI` to use `d.APIPort()`. Update `IsNodeRunning()` to still check 9099 (primary node port). The `APIPort` constant stays 9099 for CLI use — CLI always talks to primary node.

### Fix 2 — Delete Conf File After Subprocess Starts (SECURITY)

**Problem:** `yggdrasil-meshnet.conf` contains private key in plaintext, stays on disk.

**Fix:** In `core/yggservice.go`, after `IsRunning()` confirms subprocess started, delete the conf file:

```go
os.Remove(s.cfgPath) // key is loaded in memory, file no longer needed
```

### Fix 3 — Hide --identity Flag From Production Help

**Problem:** `--identity` flag lets anyone create unlimited identities. Fine for dev, dangerous for production users.

**Fix:** In `cli/cli.go` `cmdStart`, define the flag but don't include it in the Usage function's printed output. Keep it functional for devs who know about it.

### Fix 4 — One Name Per Key In store.go

**Problem:** One key can register unlimited different names, enabling name hoarding.

**Fix:** In `dht/store.go` `Put()`, before storing, check:

```go
for _, existing := range s.records {
    if existing.PublicKey == record.PublicKey &&
       existing.Name != record.Name &&
       !existing.IsExpired() {
        return fmt.Errorf("key already registered as %q — one name per identity", existing.Name)
    }
}
```

### Fix 5 — RegisterOptions Needs TTL Field

**Problem:** Pairing records need 10-minute TTL, not default 1-hour TTL. `RegisterOptions` has no TTL field.

**Fix:** In `dht/register.go`:

```go
type RegisterOptions struct {
    Name       string
    Address    string
    Services   []string
    GroupKey   string
    PrivateKey ed25519.PrivateKey
    TTL        time.Duration // 0 = use default RecordTTL (1 hour)
}
```

In `CreateRecord()`:
```go
ttl := opts.TTL
if ttl == 0 {
    ttl = RecordTTL
}
record := Record{
    ...
    Expires: time.Now().Add(ttl).Unix(),
}
```

### Fix 6 — DHT Needs privKey For Pairing

**Problem:** Pairing endpoints in API need to sign records but DHT struct has no private key.

**Fix:** Add to `dht/dht.go` DHT struct:
```go
privKey ed25519.PrivateKey
```

Add setter:
```go
func (d *DHT) SetPrivKey(privKey ed25519.PrivateKey) {
    d.privKey = privKey
}
```

In `cli/cli.go` `cmdStart`, after `d.Start()`:
```go
d.SetPrivKey(node.PrivateKey())
```

### Fix 7 — NodeIDFromBytes Missing

**Problem:** `dht.NodeIDFromBytes(pubKey)` called in pairing code but doesn't exist.

**Fix:** In `dht/routing.go`:
```go
func NodeIDFromBytes(pubKey []byte) (NodeID, error) {
    if len(pubKey) != 32 {
        return NodeID{}, fmt.Errorf("public key must be 32 bytes, got %d", len(pubKey))
    }
    var id NodeID
    copy(id[:], pubKey)
    return id, nil
}
```

---

## Pairing System — What's Built And What's Needed

### What's Built

`pairing/contacts.go` — complete. `Contact` struct, `ContactBook`, `LoadContacts()`, `Save()`, `Add()`, `All()`, `FindByName()`, `FindByAddress()`.

`pairing/pairing.go` — complete. `GenerateCode()`, `PairingRecordName()`, `PairingResponseName()`, `ContactInfo` struct.

`dht/api.go` — three pairing endpoints added: `/pair/initiate`, `/pair/poll`, `/pair/join`.

`cli/cli.go` — `cmdPair()` and `cmdContacts()` written. `pair` and `contacts` added to command switch and help text.

### How Pairing Works

```
Device A runs: meshnet pair
  → API POST /pair/initiate
  → DHT creates record keyed "MESH-XXXX" with A's address + name
  → CLI polls GET /pair/poll?code=MESH-XXXX every 2s
  → Prints code, waits up to 5 minutes

Device B runs: meshnet pair MESH-XXXX
  → API POST /pair/join?code=MESH-XXXX
  → DHT looks up "MESH-XXXX" → finds A's record
  → DHT creates response record "MESH-XXXX:response" with B's address + name
  → Returns A's info to CLI
  → B saves A to contacts.json

Device A's poll finds "MESH-XXXX:response"
  → Extracts B's info
  → A saves B to contacts.json
  → Both devices paired
```

Pairing records have 10-minute TTL. No permanent DHT pollution.

### Pairing Data Encoding

Name stored in Services field as `"pairing-name:alice"` — avoids needing a new record type, works with existing DHT infrastructure.

### After Pairing

`meshnet lookup alice` checks `contacts.json` first — instant, no DHT query needed. Falls back to DHT if not in contacts.

`meshnet contacts` lists all paired devices with address and time since pairing.

---

## Security Issues — In Priority Order

### Must Fix Before Real Users

1. **Plaintext private key in conf file** — Fix 2 above. Delete after subprocess starts.
2. **Plaintext private key in identity.json** — Needs OS keychain or password encryption. Medium complexity.
3. **No local API authentication** — Any local process can control the node. Fix: random token written to file, required in requests.
4. **STORE rate limiting** — Unlimited records accepted from any node. Fix: token bucket per source IP.

### Important But Not Blocking

5. **No peer authentication** — Nodes aren't cryptographically proven to own their claimed ID. Fix: challenge-response during ping.
6. **Eclipse attack** — Malicious nodes could fill routing table. Fix: bucket diversity enforcement.
7. **Name conflict ambiguity** — Two people named "alice" = non-deterministic lookup. Needs design decision.

---

## Sybil Attack — The Multiple Identity Problem

**What it is:** The `--identity` flag lets anyone create unlimited fake identities. This enables:
- Name squatting (register every useful name with throwaway identities)
- DHT flooding (spam routing tables with fake nodes)
- Record flooding (exhaust other nodes' storage)

**Current mitigations:**
- One name per key (Fix 4 above) — prevents hoarding with one identity
- `--identity` hidden from production help — reduces casual abuse

**Long-term solutions (not yet built):**
- Proof of work on record registration — makes bulk creation expensive
- Minimum uptime before announcing — fresh nodes can't immediately flood
- Rate limiting per public key — one key, limited announcements per hour

**Philosophy:** Full Sybil resistance requires either proof of work, proof of stake, or centralized vetting. For v1, make abuse inconvenient rather than cryptographically impossible. Full solution is Phase 6+ work.

---

## Phase Roadmap

### Phase 3 ✅ DONE
DHT, naming, lookup, announce, persistence, re-announcement, CLI, local API, peer persistence.

### Phase 3.5 ✅ DONE
TUN integration. Yggdrasil subprocess. Browser access to Yggdrasil internet. Unified identity. OS routing working.

### Phase 4 — Pairing (CURRENT — 80% DONE)
**Remaining:** Apply fixes 1-7 above, test two-node pairing on same machine, test across two real machines.

**Test sequence:**
```
Terminal 1: meshnet start --name alice --tun
Terminal 2: meshnet start --name bob --port 9002 --identity identity2.json
Terminal 3: meshnet pair          ← alice generates code
Terminal 4: meshnet pair MESH-XXXX ← bob joins
            meshnet contacts       ← both show each other
            meshnet lookup alice   ← instant from contacts
```

### Phase 5 — DNS Resolver
`.mesh` names work in browser without any configuration.

Local DNS stub resolver on `127.0.0.1:53`. Intercepts `.mesh` queries, resolves via DHT, forwards everything else to normal DNS. OS automatically configured to use local resolver.

Library: `miekg/dns` — ~100 lines of Go.

New file: `dns/resolver.go`
New CLI flag: `meshnet start --dns`

```
User types: alice.mesh in Chrome
Browser asks DNS → local resolver intercepts
Resolver does DHT lookup for "alice"
Returns Alice's Yggdrasil IPv6 address
Browser connects through TUN → mesh → Alice
```

### Phase 6 — OS Integration and Installer
- Windows Service registration (start on boot, run silently)
- System tray icon with status and quit
- Single installer: WinTun driver + yggdrasil.exe + meshnet.exe + identity setup
- Proper OS config directories (`%APPDATA%\MeshNet`)
- Encrypt identity.json with Windows Credential Manager
- Auto-enable TUN when running as service (no --tun flag needed)
- Fix TUN retry noise (proper WinTun adapter cleanup)

### Phase 7 — UI
- Desktop: system tray app, contact list, pairing flow, online/offline status
- Mobile: Android/iOS background service, simple UI, QR code scanning
- Expand local API for UI consumption

---

## How MeshNet Compares To Existing Yggdrasil Tools

**Alfis** — blockchain DNS for `.ygg` domains. Public only. Growing blockchain everyone must sync. No private groups. No service discovery. No pairing.

**Meshname** — not real naming. Base32-encodes Yggdrasil address. `200x7ae5...mesh` ≠ human name.

**Community DNS resolvers** — four servers run by individuals. Manually configure DNS to use them. Centralized. If maintainers stop, dies.

**Yggdrasil services page** — GitHub wiki. Manually updated. Raw IPv6 addresses.

**What MeshNet adds that nothing else has:**
- Human-chosen names with cryptographic ownership
- Private groups invisible to non-members  
- Service discovery
- Device pairing with contacts
- Works without configuration
- Designed for non-technical people

The gap is real. The DHT is not redundant.

---

## Known Bugs To Fix

### DHT Port Still Shows 9001
The `DHTPort` constant in `dht/dht.go` was changed to 9002 but compiled binary sometimes still shows 9001. Ensure `const DHTPort = 9002` is saved and rebuild.

### tls:// Double Prefix
In `core/node.go` one peer has `"tls://tls://62.210.85.80:39575"` — fix to `"tls://62.210.85.80:39575"`.

### TUN Retry Noise
Every startup: TUN creation fails once, retries, succeeds. WinTun driver state issue. Cosmetic — always works — but ugly. Proper fix in Phase 6 installer.

---

## Go Packages Used

```
github.com/yggdrasil-network/yggdrasil-go v0.5.12   — mesh routing
github.com/gologme/log v1.3.0                       — Yggdrasil logger interface
```

Standard library only for everything else — no external CLI, HTTP, or crypto dependencies beyond what Go ships.

---

## Important Code Locations

**Start the whole thing:** `cli/cli.go` → `cmdStart()`

**Identity loading:** `core/identity.go` → `loadOrCreateIdentity()`

**Yggdrasil subprocess:** `core/yggservice.go` → `Start()`

**DHT record storage:** `dht/store.go` → `Put()`

**Name lookup:** `dht/lookup.go` → `LookupValue()`

**Pairing initiation:** `dht/api.go` → `/pair/initiate` handler

**Contacts:** `pairing/contacts.go` → `ContactBook`

---

## The One Rule That Never Changes

Yggdrasil source code is never modified. It is a dependency. Everything is achievable from outside through configuration and wrapping. Update via `go.mod` only.

---

## Environment Variables (Dev/Testing Only)

```
IDENTITY=identity2.json    override identity file path
PORT=9002                  override DHT port (same as --port flag)
```

---

## Test Addresses (Live Yggdrasil Nodes)

```
ping 21e:e795:8e82:a9e2:ff48:952d:55f2:f0bb   ← France, responsive
ping 20c:c3d2:8c38:a030:9d87:93db:53f7:df79   ← Czechia, responsive

Browser:
http://[21e:e795:8e82:a9e2:ff48:952d:55f2:f0bb]  ← Yggdrasil map
http://[21e:a51c:885b:7db0:166e:927:98cd:d186]   ← Yggdrasil web directory
http://[200:b48d:469e:c7c7:3e13:c41d:ba4d:d2b8]  ← Yggdrasil search engine
```

---

## Session History Summary

**Session 1** — Yggdrasil embedded, permanent identity, bootstrap peers, project structure.

**Session 2** — Full Kademlia DHT: routing table, record store, RPC protocol, iterative lookup, announce. Two-node test proven working. Vision clarified as parallel internet.

**Session 3** — Phase 3 remaining work: peer persistence, re-announcement, bootstrap system, local HTTP API, full CLI. Two-node lookup working. DHT port conflict fixed. Daemon/CLI separation clean.

**Session 4 (this session)** — TUN integration: Yggdrasil subprocess, WinTun driver, identity unification, browser access working. Bootstrap peers updated (lhc.network dead, new peers from publicpeers.neilalexander.dev). Pairing system designed and 80% implemented. Security issues identified: multiple identity risk, one-name-per-key fix needed, conf file plaintext key. Docs updated.

---

*Continue in new thread. Apply all fixes listed above first, then complete and test pairing, then Phase 5 DNS resolver.*

---

## CRITICAL ARCHITECTURE FIX — Added Feb 16

### Two Yggdrasil Instances Routing Conflict

**Problem:** When running in TUN mode, both the embedded Yggdrasil library AND the subprocess were connecting to bootstrap peers with the same keypair. Remote nodes saw conflicting routing announcements from the same key and dropped all traffic. TUN adapter showed correct address and peers showed `"up": true` but `ReceivedBytes` was always 0 — packets sent but nothing returned.

**Symptom:** ping times out, curl times out, browser can't reach anything. Everything LOOKS correct (peers up, route exists, address assigned) but no traffic flows.

**Root cause confirmed by:** Running subprocess alone (`.\bin\yggdrasil.exe -useconffile yggdrasil-meshnet.conf`) — ping worked immediately. Running full MeshNet — ping failed. Same key, two instances, routing conflict.

**Fix applied:** In TUN mode, embedded library connects to NO peers. Subprocess handles all routing. Embedded library runs silently for DHT communication only.

**In `core/node.go`:** Split `Bootstrap()` into empty stub + `BootstrapPeers()` method.

**In `cli/cli.go` `cmdStart`:**
```go
if *tun {
    fmt.Println("TUN mode — mesh routing handled by subprocess")
} else {
    fmt.Println("Connecting to Yggdrasil peers...")
    node.BootstrapPeers()
    time.Sleep(3 * time.Second)
    fmt.Println("Connected to Yggdrasil mesh.")
}
```

**This fix must never be reverted.** TUN mode and embedded peer connections cannot coexist.