# MeshNet — Project Idea Document
> A truly decentralized, user-friendly private network. No company. No servers. No trust required.

*Document version 2.0 — technical decisions locked*

---

## The Problem

The internet today is broken for true peer-to-peer communication in three specific ways.

**1. NAT killed direct connectivity.** IPv4 address exhaustion forced every home and mobile device behind a router using NAT. Your device has no real public address. Nobody can reach you directly. You are hidden.

**2. Every solution reintroduces centralization.** ZeroTier and Tailscale solve NAT elegantly but replace it with a dependency on company-owned coordination servers. If the company goes down, gets hacked, gets acquired, or decides to surveil you — your network goes with it.

**3. Truly decentralized tools are unusable.** Yggdrasil solves the hard problems correctly but requires editing config files, manually finding peers from a GitHub list, and enough networking knowledge to understand what you're doing. Normal people cannot use it.

The gap: **nothing exists that is simultaneously truly decentralized, NAT-resistant, and usable by a non-technical person.**

---

## What Already Exists (And Why We Don't Rebuild It)

**Yggdrasil** is an open-source encrypted mesh network with one brilliant property — your network address is derived mathematically from your cryptographic public key. Nobody assigned it to you. It is permanent, globally unique, and entirely self-sovereign. Routing is fully distributed. There are no central servers. End-to-end encryption is built in at the protocol level.

Yggdrasil solves the hardest problems: identity, routing, encryption, and decentralization. We do not rebuild these. We build on top of them.

---

## What We Are Building

A three-layer wrapper on top of Yggdrasil that makes it accessible and private.

---

## The Architecture

```
┌─────────────────────────────────────┐
│  Layer 4 — Application              │
│  Clean UI. QR pairing. No config.   │
├─────────────────────────────────────┤
│  Layer 3 — Distributed Name System  │
│  DHT-based registry within the mesh │
│  Human names → Yggdrasil addresses  │
│  Private group scoping              │
├─────────────────────────────────────┤
│  Layer 2 — Bootstrap                │
│  Hardcoded community node list      │
│  First contact only. Then DHT takes │
│  over. No permanent dependency.     │
├─────────────────────────────────────┤
│  Layer 1 — Yggdrasil (unchanged)    │
│  Identity, routing, encryption.     │
│  We do not touch this.              │
└─────────────────────────────────────┘
```

---

## Layer by Layer — What Each Does and Why

### Layer 1 — Yggdrasil
**What it does:** Generates your permanent cryptographic identity and address. Routes packets across the mesh. Encrypts everything end-to-end.

**Why we don't touch it:** It is battle-tested, open-source, and maintained. Every improvement they make, we get for free. Rebuilding this is months of work to arrive at something worse.

### Layer 2 — Bootstrap
**What it does:** Solves the cold-start problem. When you first install and have no peers, you need to find the network somehow. A small hardcoded list of community-run stable nodes (10–20 addresses across different people and countries) is baked into the installer. You only need one to be alive to connect. After first contact you are in the mesh via DHT and never need bootstrap nodes again.

**Why this minimal centralization is acceptable:** You need it exactly once. After that it is irrelevant. Bitcoin uses the same pattern with its seed nodes. It is the irreducible minimum and it is manageable.

### Layer 3 — Distributed Name System (The Novel Work)
**What it does:** Runs a lightweight DHT (Kademlia algorithm) entirely within the Yggdrasil network. Provides three things:

- **Human readable names** — maps "alice" to her Yggdrasil address so you never handle cryptographic addresses manually
- **Peer discovery** — nodes announce presence to the DHT so others can find them
- **Private group scoping** — a group generates a shared key. Your entry in the DHT is only visible to nodes that can prove group membership. To everyone else on the global Yggdrasil mesh, you are invisible.

**Why it is not just DNS:** Traditional DNS is centralized — ICANN controls roots, ISPs run resolvers. This registry lives entirely within the mesh itself, distributed across participating nodes. There is no registry server. There is no authority. The data exists as long as nodes exist.

**Why DHT and not blockchain:** No consensus overhead, no tokens, no mining, no latency. DHT is fast, proven, and simple. BitTorrent has used it at massive scale for twenty years.

### Layer 4 — Application
**What it does:** Hides all of the above. The user sees: their device name, a list of people in their network with online/offline status, a button to add someone (generates a QR code or short pairing code), and connection quality. That is all.

**What the user never sees:** Config files. IPv6 addresses. DHT internals. Peer lists. Yggdrasil anything.

---

## What This Solves That Others Don't

| Problem | ZeroTier | Tailscale | Yggdrasil | MeshNet |
|---|---|---|---|---|
| No central servers | ✗ | ✗ | ✓ | ✓ |
| Works through NAT | ✓ | ✓ | Partial | ✓ |
| Permanent self-owned identity | ✗ | ✗ | ✓ | ✓ |
| Normal person can use it | ✓ | ✓ | ✗ | ✓ |
| Private groups | ✓ | ✓ | ✗ | ✓ |
| Survives company shutdown | ✗ | ✗ | ✓ | ✓ |

---

## What We Are NOT Building (Scope Boundaries)

- We are not rebuilding Yggdrasil or forking it
- We are not building anonymity features (that is Tor's problem)
- We are not building transport agnosticism in v1 (WiFi direct, Bluetooth, LoRa are future work)
- We are not building a general internet replacement — this is a private network tool

---

## Who This Is For

- Developers and technical people who don't want to trust a company with their infrastructure
- Small teams and organizations wanting a private network they fully own
- People in regions where ZeroTier's servers could be blocked or compelled
- Anyone who values genuinely owning their own network the way people are starting to value owning their own data

---

## Build Sequence

**Phase 1 — Foundation**
Get Yggdrasil running as an embedded Go library. Understand key generation, config, and peer connection. Write a thin wrapper that starts Yggdrasil inside your own process and exposes a clean internal API.

**Phase 2 — Bootstrap**
Build the bootstrap node list and connection logic. A new install should find the network automatically with zero user configuration.

**Phase 3 — DHT Name Registry**
Implement Kademlia DHT over Yggdrasil connections. Define the schema for name registration and lookup. Implement private group scoping with shared key gating.

**Phase 4 — Pairing System**
Build QR code and short-code pairing. This is the key exchange — two devices exchange Yggdrasil public keys and register each other in their shared group DHT namespace.

**Phase 5 — Application**
Desktop menubar app (Electron or native). Mobile (React Native or Flutter). Show network members, online status, latency. Make pairing dead simple.

---

## Technical Decisions — Locked

These decisions were made deliberately and should not be revisited without strong reason.

### Language — Go

The entire codebase is Go. Not Python, not JavaScript, not Rust. Go specifically because Yggdrasil is written in Go and its library interface is native Go. Same language means direct function calls, same types, same memory space. No translation layer. No friction. Go's concurrency model — goroutines and channels — is also exactly the right tool for network code that manages many simultaneous peer connections.

### Yggdrasil Integration — Library, Not Subprocess

This is one of the most important decisions in the project. Yggdrasil is embedded directly as a Go library, not run as a separate process alongside your application.

**What subprocess would mean:** Two separate programs. Your app starts Yggdrasil as a background process and communicates with it by sending JSON messages back and forth over a local socket. You manage two programs, handle crashes independently, deal with startup timing, ship two binaries.

**What library means:** One program. Yggdrasil's code compiles directly into your binary. You call its functions like your own code. One binary ships to the user. Yggdrasil is invisible inside it.

```
Subprocess (rejected):
Your App  →  JSON over socket  →  Yggdrasil process
Your App  ←  JSON over socket  ←  Yggdrasil process
Two programs. A window between them. Clunky.

Library (chosen):
Your App
  └── Yggdrasil (embedded)
        └── node.Start()
        └── node.GetAddress()
        └── node.Dial(peer)
One program. Direct calls. Clean.
```

The library approach is strictly better in every dimension — simpler architecture, single binary distribution, no inter-process communication overhead, no subprocess babysitting, no risk of the two programs getting out of sync.

The only reason to choose subprocess is if your application is written in a different language than the library. Since we're in Go, that reason doesn't exist.

### Yggdrasil Itself — Untouched, Ever

Yggdrasil's source code is never modified. It is a dependency, not a component. We consume it exactly as its team ships it. When they release updates, security fixes, or performance improvements, we update a version number in go.mod and get everything for free. If we modified their code, we'd have to manually reconcile every upstream change forever. That maintenance burden compounds and eventually kills projects.

The rule is simple: if something needs to change in how Yggdrasil behaves, we find a way to achieve it from the outside through configuration or wrapping. We never change it from the inside.

### Project Structure — Go

```
your-meshnet/
│
├── main.go              — entry point, wires everything together
│
├── core/
│   └── node.go          — embeds Yggdrasil as library
│                          owns the node instance
│                          exposes clean internal API to rest of app
│
├── dht/
│   └── registry.go      — Kademlia DHT name registry
│                          runs over Yggdrasil connections
│                          handles names, discovery, private groups
│
├── pairing/
│   └── pairing.go       — key exchange logic
│                          generates pairing codes and QR
│                          handles handshake between two devices
│
└── app/
    └── app.go           — top level application
                           ties all layers together
                           handles user-facing logic
```

One binary. Everything inside. User installs it and the entire stack — Yggdrasil included — is just there, invisibly running.

---

## The One-Sentence Pitch

> Everything ZeroTier does, with no company in the middle, no server to trust, and an interface simple enough for anyone.

---