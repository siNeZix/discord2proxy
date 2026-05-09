package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DataDir      string
	ProxyFile    string
	ProxyPorts   []int
	DiscordPaths []string
	DLLFiles     []string
}

func Default() *Config {
	exe, _ := os.Executable()
	dataDir := filepath.Join(filepath.Dir(exe), "data")

	localAppData := os.Getenv("LocalAppData")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	return &Config{
		DataDir:   dataDir,
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

func (c *Config) ProxyFilePath() string {
	return filepath.Join(c.DataDir, c.ProxyFile)
}

func (c *Config) DLLSourcePath(name string) string {
	return filepath.Join(c.DataDir, name)
}
