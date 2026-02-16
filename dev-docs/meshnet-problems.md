# MeshNet — Known Problems
> Every problem with the current version. No mercy. No sugarcoating.

*Last updated: Phase 3 + TUN integration complete.*

---

## RESOLVED — Fixed In This Version

### ✅ No TUN Adapter
Fixed. Yggdrasil subprocess creates TUN via WinTun driver. Browser can reach Yggdrasil addresses. Ping works to mesh nodes.

### ✅ Bootstrap Peers Dead (lhc.network)
Fixed. All lhc.network peers replaced with current live nodes from publicpeers.neilalexander.dev with 100% uptime.

### ✅ Two Identities (Partial Fix)
Partially fixed. MeshNet reads installed Yggdrasil keypair if present. When running subprocess mode, same private key used for both embedded routing and subprocess TUN. One address everywhere.

---

## CRITICAL — Breaks Core Functionality

### 1. TUN Adapter Retry Noise Every Startup
**What it is:** Every startup the Yggdrasil subprocess fails TUN creation once with "Cannot create a file when that file already exists", removes the driver, reinstalls, then succeeds. Takes 15-20 seconds of noisy output.

**Why it happens:** WinTun driver state from previous run is not fully cleaned up by our `netsh delete` call before the subprocess can use it. Windows driver timing issue.

**Why it's not fatal:** Yggdrasil always recovers automatically. TUN always ends up working. Functional impact is startup delay only.

**Fix:** Proper cleanup using WinTun's own API to release the adapter before relaunching. Or track the adapter name Yggdrasil assigns (it uses "auto") and delete that specific adapter. Medium complexity.

---

### 2. Bootstrap DHT Is Empty
**What it is:** `bootstrap_peers.go` contains an empty list. Fresh node with no `peers.json` starts in isolated mode.

**Why it matters:** Every new user hits this. They install MeshNet, run it, DHT finds nobody. Unusable until manually adding a peer.

**Fix:** Run at least one permanent MeshNet bootstrap node. Infrastructure problem as much as code problem.

---

### 3. DHT Has No Other Nodes
**What it is:** Zero other MeshNet nodes exist on the network. Every lookup returns nothing.

**Why it matters:** Network of one is not a network. Chicken-and-egg adoption problem.

**Fix:** Get other people running MeshNet. Requires Phase 4 (pairing) and Phase 7 (real UI) to make adoption possible.

---

## SERIOUS — Significant Limitations

### 4. Private Key Stored Plaintext
**What it is:** `identity.json` stores ed25519 private key as plaintext hex.

**Why it matters:** Anyone who reads this file permanently steals your identity. No recovery — the key IS the identity.

**Fix:** Encrypt with user password or OS keychain before real users.

---

### 5. No Peer Authentication
**What it is:** When a node claims to be ID `abc123`, we believe it. No cryptographic proof.

**Why it matters:** Malicious nodes could poison routing tables, redirect lookups.

**Fix:** Challenge-response during ping handshake. Sign a nonce with private key, verify against claimed public key.

---

### 6. No Lookup Caching
**What it is:** Every lookup starts a fresh iterative DHT query. No results cached.

**Why it matters:** Slow for repeated lookups. In browser DNS context would be noticeable on every page load.

**Fix:** In-memory cache with TTL matching record expiry. Low complexity.

---

### 7. Bucket Full = Silent Drop
**What it is:** When a Kademlia bucket fills to K=20, new contacts are silently dropped instead of LRU eviction with liveness check.

**Why it matters:** Routing table stagnates with offline nodes as network grows.

**Fix:** Ping oldest contact first, replace if dead. Standard Kademlia behavior.

---

### 8. Single Connection Per RPC
**What it is:** Every DHT message opens a new TCP connection. No connection pooling.

**Why it matters:** High overhead on high-latency connections. Each lookup = multiple TCP handshakes.

**Fix:** Connection pool per peer. Not urgent until real network traffic exists.

---

### 9. No Name Conflict Resolution
**What it is:** Two different keys registering `alice` results in non-deterministic lookup depending on which DHT node is asked.

**Why it matters:** Name collisions become real as network grows.

**Fix:** Needs design decision — full pubkey suffix, trust model, or first-registered-globally consensus.

---

### 10. No Group Key Rotation
**What it is:** Private group keys are static. No rotation mechanism.

**Why it matters:** Leaked group key = permanent visibility. No forward secrecy.

**Fix:** Versioned group keys with rotation protocol. Phase 4+ work.

---

## MODERATE — Annoyances and Missing Features

### 11. peers.json and identity.json in Working Directory
Should be in OS config dirs (`%APPDATA%\MeshNet` on Windows). Low complexity.

### 12. No PATH Installation
Must run `.\meshnet.exe` from project dir. No installer. Phase 6 work.

### 13. Lookup Requires Running Node
`meshnet lookup` requires a node already running. Can't do a quick one-shot lookup.

**Fix:** Ephemeral lookup-only mode that starts minimal node, does lookup, exits.

### 14. No Progress Feedback During Lookup
10-second timeout with no output feels like the app froze.

**Fix:** Print progress lines during iterative lookup. Low complexity.

### 15. No meshnet unregister Command
Names linger up to 1 hour after stopping re-announcement. No way to immediately remove your record.

**Fix:** Send signed deletion record to K nearest nodes.

### 16. Services Not Verified
Announcing `ssh:22` doesn't check anything is actually listening there.

**Fix:** Optional port verification flag during lookup. Low complexity.

### 17. Admin Socket Created But Unused
Yggdrasil admin socket available but never queried for peer/routing info.

**Fix:** Query for Yggdrasil peer count in `meshnet status`. Low complexity.

### 18. No Logging System
Bare `fmt.Println` everywhere. No levels, no timestamps, no log files.

**Fix:** Structured logging with levels. Write to file when running as service.

### 19. No Version Command
`meshnet --version` doesn't exist.

**Fix:** Embed version at build time with `-ldflags`. One hour of work.

### 20. No Contacts System
No way to save paired devices locally. Lookup always goes to DHT.

**Fix:** `contacts.json` — local address book. This is Phase 4 pairing work.

### 21. --tun Flag Required Every Time
Users have to remember to pass `--tun` for browser access. Annoying.

**Fix:** Auto-detect if running as admin and enable TUN automatically. Or make TUN the default when admin. Low complexity.

---

## ARCHITECTURE — Design Debt

### 22. Two Yggdrasil Instances Running Simultaneously
Both embedded library and subprocess connect to the mesh. Both consume bandwidth for peer connections. Slightly wasteful.

**Ideal fix:** Phase 6 — MeshNet manages TUN itself, embedded library not needed for routing. One process.

### 23. No Record Size Limits
Store accepts arbitrarily large records. Memory exhaustion vector.

**Fix:** Hard limits on field sizes. Trivial to add.

### 24. Goroutine Leak In LookupValue
In-flight goroutines can't be cancelled when lookup succeeds early.

**Fix:** `context.Context` with cancellation throughout. Medium complexity.

### 25. No Metrics
No way to measure DHT health, lookup latency, peer count over time.

**Fix:** Internal counters in `/status` API response. Low complexity.

---

## SECURITY — Must Fix Before Real Users

### 26. Local API Has No Authentication
`http://127.0.0.1:9099` accepts requests from any local process.

**Fix:** Random auth token written to local file, required in CLI requests.

### 27. DHT Records Can Be Flooded
Unlimited STORE requests accepted. CPU and memory exhaustion vector.

**Fix:** Rate limiting per source address. Token bucket. Low complexity.

### 28. No Protection Against Eclipse Attack
Malicious nodes could fill routing table and control all lookups.

**Fix:** Bucket diversity enforcement, proper LRU eviction, diverse peer sources.

### 29. Yggdrasil Subprocess Config Contains Private Key In Plaintext
`yggdrasil-meshnet.conf` is written to disk with the private key in it.

**Why it matters:** Any process that reads that file gets the private key.

**Fix:** Delete the config file immediately after subprocess starts, or use a pipe/stdin instead of a file. Low complexity — file only needs to exist during startup.

---

## ECOSYSTEM — Context

### Is The DHT Useful Given Existing Yggdrasil Services?

Yes. Here is the precise comparison with what already exists:

**Alfis** — blockchain DNS for `.ygg`. Public only. Requires syncing a growing blockchain. No private layer. No service discovery. No pairing.

**Meshname** — not real naming. Base32-encodes your address into the name. `200x7ae5...mesh` is not a human name.

**Community DNS resolvers** — four servers run by individuals. Centralized. Require manual DNS config. One person stops maintaining, service dies.

**Services page** — a GitHub wiki. Manually updated. Raw IPv6 addresses. No automation.

**What MeshNet adds that nothing else has:**
- Human-chosen names with cryptographic proof of ownership
- Private groups — invisible to non-members
- Service discovery
- Device pairing
- Works without any configuration changes
- Designed for non-technical people

The gap is real. The DHT is not redundant with anything that exists.

---

## SUMMARY

| Severity | Count | Blocking Production? |
|---|---|---|
| Critical | 3 | #2 (no bootstrap nodes) blocks adoption |
| Serious | 7 | #4 (plaintext key) before real users |
| Moderate | 11 | No — quality work |
| Architecture | 4 | No — accumulates as debt |
| Security | 4 | #26, #27, #29 before real users |

**Minimum before real users:**
1. At least one bootstrap node running
2. Encrypt identity.json
3. Delete yggdrasil-meshnet.conf after subprocess starts
4. Add local API authentication
5. Add STORE rate limiting