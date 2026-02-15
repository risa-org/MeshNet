# bin/

This folder contains the Yggdrasil binary required for TUN mode.
It is not committed to git.

## Setup

Download Yggdrasil v0.5.12 for your platform:
https://github.com/yggdrasil-network/yggdrasil-go/releases/tag/v0.5.12

Extract yggdrasil.exe and wintun.dll into this folder.

Then run:
    meshnet start --name yourname --tun
```

---

## Final Tree After Cleanup
```
meshnet/
├── .gitignore
├── go.mod
├── go.sum
├── main.go
├── bin/
│   └── README.md          ← committed
│   (yggdrasil.exe)        ← NOT committed
│   (wintun.dll)           ← NOT committed
├── cli/
│   └── cli.go
├── core/
│   ├── cert.go
│   ├── identity.go
│   ├── node.go
│   └── yggservice.go
└── dht/
    ├── announce.go
    ├── api.go
    ├── bootstrap_peers.go
    ├── dht.go
    ├── lookup.go
    ├── peers.go
    ├── register.go
    ├── routing.go
    ├── rpc.go
    └── store.go
```

Clean. Everything that shouldn't be in git is gitignored. Everything that should be tracked is there.

Run the cleanup, rebuild, test once more with the new bin path, then commit:
```
go build -o meshnet.exe .
.\meshnet.exe start --name alice --tun
git add .
git commit -m "chore: clean project structure, update peers, gitignore"
git push