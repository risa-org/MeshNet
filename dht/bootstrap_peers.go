package dht

// meshnetBootstrapPeers is a list of well-known long-running MeshNet nodes
// used when a node starts fresh with no saved peers
// format: [yggdrasil-address]:port
//
// These are community-run nodes that agree to be always-on entry points
// Anyone can run a bootstrap node — just add it here and submit a PR
// A node that goes offline is simply skipped during bootstrap
var meshnetBootstrapPeers = []string{
	// placeholder — replace with real community bootstrap nodes
	// when the network has participants
	// e.g. "[200:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx:xxxx]:9001"
}
