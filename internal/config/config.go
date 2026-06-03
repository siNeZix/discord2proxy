package config

import (
	"os"
	"path/filepath"

	"discord-szx/internal/assets"
)

// AppName is the technical product name, used for file/process names.
const AppName = "discord2proxy-cli"

// DisplayName is the human-facing product name shown in the GUI.
const DisplayName = "Прокси для Discord"

// Repository coordinates used for the GitHub Releases update check.
const (
	RepoOwner = "siNeZix"
	RepoName  = "discord2proxy"
)

// Version is the application version (without a leading "v"). It defaults to
// "0.1.0-dev" and is overridden at build time via:
//
//	-ldflags "-X discord-szx/internal/config.Version=<value>"
var Version = "0.1.0-dev"

// VersionTag returns the version with a leading "v", e.g. "v0.1.0".
func VersionTag() string {
	return "v" + Version
}

// Title returns the product name with version, e.g. "discord2proxy v0.1.0".
func Title() string {
	return AppName + " " + VersionTag()
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
