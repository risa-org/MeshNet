# Contributing to MeshNet

MeshNet is in active early development. The core mesh routing and naming layer works — pairing, DNS, and the installer are still being built.

## Current State

**What works today:**
- Yggdrasil mesh integration with TUN support
- Kademlia DHT — name registration, lookup, peer routing
- Cryptographic identity and record signing
- Peer persistence and re-announcement

**In progress:**
- Device pairing via short codes
- DNS resolver for `.mesh` names
- Windows Service and installer

**Not started:**
- Mobile apps
- Cross-platform support (Linux, macOS)
- UI beyond the CLI

---

## How To Help Right Now

The most useful things you can do today, before formal contributions open:

**Run a node and report what breaks.**
Two-node testing is the most valuable thing. If you have two machines, run a node on each, connect them with `meshnet peer add`, and see if lookup works across them. Open an issue with what you find.

**Run a bootstrap node.**
If you have a stable public server with a static IP, running a permanent MeshNet node would unblock a lot of testing. Open an issue titled "Bootstrap node offer" with your server's specs and location.

**Review the architecture.**
Read through `dev-docs/` and the code. If you spot a design flaw, a security issue, or a simpler approach to something, open an issue. Early architectural feedback is far more valuable than code at this stage.

**Test on non-Windows platforms.**
The codebase is written in Go and should mostly work on Linux and macOS. The TUN code is Windows-specific but everything else should compile. If you try it and hit issues, open an issue.

---

## When Will Contributions Be Accepted?

Formal PRs will be accepted once:

- Device pairing is complete and tested across real machines
- At least one permanent bootstrap node is running
- DHT is validated on a multi-node network

There's no fixed date for this. Check the [dev branch](../../tree/dev) to see what's currently being built.

---

## Future Contribution Areas

Once the core is stable, we'll need help with:

| Area | Skills Needed |
|------|--------------|
| DNS resolver (`.mesh` domains) | Go, DNS protocol |
| Linux/macOS TUN support | Go, platform networking |
| Windows Service + system tray | Go, Windows APIs |
| Installer | NSIS or similar |
| Mobile apps | React Native or Flutter |
| Bootstrap node hosting | Linux sysadmin |
| Documentation | Technical writing |
| Security audit | Cryptography, P2P security |

---

## Development Setup

```bash
git clone https://github.com/risa-org/MeshNet
cd MeshNet

# Download Yggdrasil v0.5.12 binary:
# https://github.com/yggdrasil-network/yggdrasil-go/releases/tag/v0.5.12
# Place yggdrasil.exe and wintun.dll in bin/

go build -o meshnet.exe .

# Run as Administrator (required for TUN adapter)
.\meshnet.exe start --name yourname --tun
```

See `README.md` for full usage.

---

## Reporting Issues

When opening an issue please include:

- OS and version
- Go version (`go version`)
- Full output of the command that failed
- What you expected vs what happened

For security issues — open a regular issue. There's no sensitive user data at risk in the current version and the project is not yet in production use.

---

## Code Style

Standard Go conventions. Run `gofmt` before submitting anything. No external dependencies beyond what's already in `go.mod`.

---

## Questions

Open an issue. Label it with a question mark in the title if you want. We're happy to discuss design decisions, architecture, or anything else.