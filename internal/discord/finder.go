package discord

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"

	"discord-szx/internal/config"
)

type DiscordInstall struct {
	Path    string
	Version string
	Channel string
	AppDir  string
}

var channelOrder = map[string]int{
	"stable":      0,
	"ptb":         1,
	"canary":      2,
	"development": 3,
}

func channelFromPath(p string) string {
	base := filepath.Base(p)
	switch strings.ToLower(base) {
	case "discord":
		return "stable"
	case "discordptb":
		return "ptb"
	case "discordcanary":
		return "canary"
	case "discorddevelopment":
		return "development"
	default:
		return "unknown"
	}
}

func findLatestAppDir(discordDir string) (string, string, bool) {
	entries, err := os.ReadDir(discordDir)
	if err != nil {
		return "", "", false
	}

	var best string
	var bestVer string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "app-") {
			continue
		}
		exePath := filepath.Join(discordDir, e.Name(), "Discord.exe")
		if _, err := os.Stat(exePath); err != nil {
			continue
		}
		if e.Name() > bestVer {
			bestVer = e.Name()
			best = filepath.Join(discordDir, e.Name())
		}
	}
	return best, bestVer, best != ""
}

func findFromRegistry() []DiscordInstall {
	var installs []DiscordInstall

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		log.Printf("discord: cannot open registry Uninstall key: %v", err)
		return installs
	}
	defer k.Close()

	names, err := k.ReadSubKeyNames(0)
	if err != nil {
		log.Printf("discord: cannot enumerate registry subkeys: %v", err)
		return installs
	}

	for _, name := range names {
		sk, err := registry.OpenKey(k, name, registry.READ)
		if err != nil {
			continue
		}

		displayName, _, _ := sk.GetStringValue("DisplayName")
		if !strings.Contains(strings.ToLower(displayName), "discord") {
			sk.Close()
			continue
		}

		installLoc, _, err := sk.GetStringValue("InstallLocation")
		if err != nil {
			uninstallStr, _, err2 := sk.GetStringValue("UninstallString")
			if err2 != nil {
				sk.Close()
				continue
			}
			installLoc = filepath.Dir(strings.Trim(uninstallStr, `"`))
		}
		sk.Close()

		installLoc = strings.Trim(installLoc, `"`)

		channel := channelFromPath(installLoc)
		appDir, version, ok := findLatestAppDir(installLoc)
		if !ok {
			continue
		}

		installs = append(installs, DiscordInstall{
			Path:    installLoc,
			Version: version,
			Channel: channel,
			AppDir:  appDir,
		})
	}

	return installs
}

func findFromFilesystem(cfg *config.Config) []DiscordInstall {
	var installs []DiscordInstall

	for _, p := range cfg.DiscordPaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		channel := channelFromPath(p)
		appDir, version, ok := findLatestAppDir(p)
		if !ok {
			continue
		}
		installs = append(installs, DiscordInstall{
			Path:    p,
			Version: version,
			Channel: channel,
			AppDir:  appDir,
		})
	}

	return installs
}

func deduplicate(installs []DiscordInstall) []DiscordInstall {
	seen := make(map[string]bool)
	var result []DiscordInstall
	for _, inst := range installs {
		if seen[inst.AppDir] {
			continue
		}
		seen[inst.AppDir] = true
		result = append(result, inst)
	}
	return result
}

func FindDiscord(cfg *config.Config) ([]DiscordInstall, error) {
	var all []DiscordInstall

	all = append(all, findFromRegistry()...)
	all = append(all, findFromFilesystem(cfg)...)

	all = deduplicate(all)

	sort.Slice(all, func(i, j int) bool {
		ci, oki := channelOrder[all[i].Channel]
		cj, okj := channelOrder[all[j].Channel]
		if !oki {
			ci = 99
		}
		if !okj {
			cj = 99
		}
		return ci < cj
	})

	return all, nil
}

func FindPrimaryDiscord(cfg *config.Config) (*DiscordInstall, error) {
	installs, err := FindDiscord(cfg)
	if err != nil {
		return nil, err
	}
	if len(installs) == 0 {
		return nil, fmt.Errorf("Discord не найден; проверены реестр и пути: %v", cfg.DiscordPaths)
	}
	return &installs[0], nil
}

func FindDiscordByChannel(cfg *config.Config, channel string) (*DiscordInstall, error) {
	installs, err := FindDiscord(cfg)
	if err != nil {
		return nil, err
	}
	for _, inst := range installs {
		if inst.Channel == channel {
			return &inst, nil
		}
	}
	return nil, fmt.Errorf("канал Discord %q не найден", channel)
}

func FindDiscordByPath(dir string) (*DiscordInstall, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("путь Discord %s не существует: %w", dir, err)
	}
	channel := channelFromPath(dir)
	appDir, version, ok := findLatestAppDir(dir)
	if !ok {
		return nil, fmt.Errorf("папка app-x.x.x не найдена в %s", dir)
	}
	return &DiscordInstall{
		Path:    dir,
		Version: version,
		Channel: channel,
		AppDir:  appDir,
	}, nil
}
