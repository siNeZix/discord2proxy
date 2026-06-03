package assets

import _ "embed"

//go:embed DWrite.dll
var DWriteDLL []byte

//go:embed force-proxy.dll
var ForceProxyDLL []byte

//go:embed favicon-original.png
var LogoPNG []byte

// DLL filenames — single source of truth for both config (ordering/verify)
// and deploy (embedded data lookup).
const (
	DWriteName     = "DWrite.dll"
	ForceProxyName = "force-proxy.dll"
)

// DLLData maps the canonical DLL filename to its embedded bytes.
// Iteration order is undefined; use Names() for a stable deploy order.
var DLLData = map[string][]byte{
	DWriteName:     DWriteDLL,
	ForceProxyName: ForceProxyDLL,
}

// Names returns the DLL filenames in canonical deploy order.
func Names() []string {
	return []string{DWriteName, ForceProxyName}
}
