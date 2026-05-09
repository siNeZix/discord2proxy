package config

import (
	"os"
	"path/filepath"
)

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
		ProxyFile: "proxy.txt",
		ProxyPorts: []int{2080, 1080},
		DiscordPaths: []string{
			filepath.Join(localAppData, "Discord"),
			filepath.Join(localAppData, "DiscordPTB"),
			filepath.Join(localAppData, "DiscordCanary"),
			filepath.Join(localAppData, "DiscordDevelopment"),
		},
		DLLFiles: []string{"DWrite.dll", "force-proxy.dll"},
	}
}
