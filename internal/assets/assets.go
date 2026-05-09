package assets

import _ "embed"

//go:embed DWrite.dll
var DWriteDLL []byte

//go:embed force-proxy.dll
var ForceProxyDLL []byte
