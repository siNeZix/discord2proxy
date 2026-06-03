package config

import (
	"os"
	"path/filepath"

	"discord-szx/internal/assets"
)

// AppName is the user-facing product name.
const AppName = "discord2proxy"

// Version is the application version. It defaults to "1.0.0" and can be
// overridden at build time via:
//
//	-ldflags "-X discord-szx/internal/config.Version=<value>"
var Version = "1.0.0"

// Title returns the product name with version, e.g. "discord2proxy v1.0.0".
func Title() string {
	return AppName + " v" + Version
}

type Config struct {
	ProxyFile    string
	ProxyPorts   []int
	DiscordPaths []string
	DLLFiles     []string
}

func Default() *Config {
	localAppData := os.Getenv("LocalAppData")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	return &Config{
		ProxyFile:  "proxy.txt",
		ProxyPorts: []int{2080, 1080},
		DiscordPaths: []string{
			filepath.Join(localAppData, "Discord"),
			filepath.Join(localAppData, "DiscordPTB"),
			filepath.Join(localAppData, "DiscordCanary"),
			filepath.Join(localAppData, "DiscordDevelopment"),
		},
		DLLFiles: assets.Names(),
	}
}
