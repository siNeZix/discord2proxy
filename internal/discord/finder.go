package discord

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// parseAppVersion extracts numeric version components from an "app-1.0.10000"
// directory name. Returns nil if the name has no parseable version.
func parseAppVersion(name string) []int {
	raw := strings.TrimPrefix(name, "app-")
	if raw == name {
		return nil
	}
	parts := strings.Split(raw, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return nil
	}
	return nums
}

// compareVersions returns >0 if a is newer than b, <0 if older, 0 if equal.
func compareVersions(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			return -1
		}
	}
	return len(a) - len(b)
}

func findLatestAppDir(discordDir string) (string, string, bool) {
	entries, err := os.ReadDir(discordDir)
	if err != nil {
		return "", "", false
	}

	var best string
	var bestName string
	var bestVer []int
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "app-") {
			continue
		}
		exePath := filepath.Join(discordDir, e.Name(), "Discord.exe")
		if _, err := os.Stat(exePath); err != nil {
			continue
		}
		ver := parseAppVersion(e.Name())
		if ver == nil {
			continue
		}
		if best == "" || compareVersions(ver, bestVer) > 0 {
			bestVer = ver
			bestName = strings.TrimPrefix(e.Name(), "app-")
			best = filepath.Join(discordDir, e.Name())
		}
	}
	return best, bestName, best != ""
}

// exeFromCommandLine extracts the executable path from a Windows command line.
// Handles a quoted path ("C:\dir\Update.exe" --uninstall) as well as an
// unquoted path where the first ".exe" token marks the end of the path.
func exeFromCommandLine(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if end := strings.Index(cmd[1:], `"`); end >= 0 {
			return cmd[1 : 1+end]
		}
		return strings.Trim(cmd, `"`)
	}
	lower := strings.ToLower(cmd)
	if idx := strings.Index(lower, ".exe"); idx >= 0 {
		return cmd[:idx+len(".exe")]
	}
	return cmd
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
		if err != nil || strings.TrimSpace(installLoc) == "" {
			uninstallStr, _, err2 := sk.GetStringValue("UninstallString")
			if err2 != nil {
				sk.Close()
				continue
			}
			installLoc = filepath.Dir(exeFromCommandLine(uninstallStr))
		}
		sk.Close()

		installLoc = strings.Trim(strings.TrimSpace(installLoc), `"`)

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

// NormalizeChannel maps user-facing channel aliases to canonical names.
func NormalizeChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "dev", "development":
		return "development"
	case "ptb":
		return "ptb"
	case "canary":
		return "canary"
	case "stable":
		return "stable"
	default:
		return strings.ToLower(strings.TrimSpace(channel))
	}
}

func FindDiscordByChannel(cfg *config.Config, channel string) (*DiscordInstall, error) {
	channel = NormalizeChannel(channel)
	installs, err := FindDiscord(cfg)
	if err != nil {
		return nil, err
	}
	for i := range installs {
		if installs[i].Channel == channel {
			return &installs[i], nil
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
