package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"discord-szx/internal/assets"
	"discord-szx/internal/config"
	"discord-szx/internal/discord"
	"discord-szx/internal/proxy"
)

var hideWindow = &syscall.SysProcAttr{HideWindow: true}

type Deployer struct {
	Config *config.Config
	DryRun bool
	Force  bool
}

func New(cfg *config.Config, dryRun, force bool) *Deployer {
	return &Deployer{
		Config: cfg,
		DryRun: dryRun,
		Force:  force,
	}
}

// channelExeName returns the Discord process image name for a given channel.
func channelExeName(channel string) string {
	switch channel {
	case "ptb":
		return "DiscordPTB.exe"
	case "canary":
		return "DiscordCanary.exe"
	case "development":
		return "DiscordDevelopment.exe"
	default:
		return "Discord.exe"
	}
}

// isProcessRunning reports whether a process with the given image name is
// running. On a tasklist failure it fails closed (returns true) so callers
// don't deploy over a possibly-running, file-locking Discord.
func isProcessRunning(exeName string) bool {
	cmd := exec.Command("tasklist", "/NH", "/FI", "IMAGENAME eq "+exeName)
	cmd.SysProcAttr = hideWindow
	out, err := cmd.Output()
	if err != nil {
		// Fail closed: assume running rather than risk locked-file writes.
		return true
	}
	return bytes.Contains(out, []byte(exeName))
}

// IsDiscordRunning reports whether the Discord process for the given channel
// is currently running. Exposed for the GUI to poll without performing any
// deploy action.
func IsDiscordRunning(channel string) bool {
	return isProcessRunning(channelExeName(channel))
}

func formatProxyConfig(host string, port int) []byte {
	return []byte(fmt.Sprintf("SOCKS5_PROXY_ADDRESS=%s\nSOCKS5_PROXY_PORT=%d\n", host, port))
}

func (d *Deployer) Deploy(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	if IsDiscordRunning(install.Channel) && !d.Force {
		return fmt.Errorf("Discord запущен — файлы заблокированы. Закройте Discord и попробуйте снова")
	}

	if d.DryRun {
		return d.dryRun(install, proxyInfo)
	}

	content := formatProxyConfig(proxyInfo.Host, proxyInfo.Port)

	targetProxyPath := filepath.Join(install.AppDir, d.Config.ProxyFile)
	if err := writeFile(targetProxyPath, content); err != nil {
		return fmt.Errorf("ошибка записи proxy.txt в %s: %w", targetProxyPath, err)
	}
	fmt.Printf("  Written %s\n", targetProxyPath)

	for _, dll := range d.Config.DLLFiles {
		data, ok := assets.DLLData[dll]
		if !ok {
			return fmt.Errorf("неизвестный файл %s", dll)
		}
		dst := filepath.Join(install.AppDir, dll)
		if err := writeFile(dst, data); err != nil {
			return fmt.Errorf("ошибка записи %s в %s: %w", dll, dst, err)
		}
		fmt.Printf("  Written %s -> %s (%d bytes)\n", dll, dst, len(data))
	}

	return nil
}

func (d *Deployer) Verify(install *discord.DiscordInstall) error {
	files := make([]string, 0, len(d.Config.DLLFiles)+1)
	files = append(files, d.Config.DLLFiles...)
	files = append(files, d.Config.ProxyFile)
	for _, name := range files {
		p := filepath.Join(install.AppDir, name)
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("проверка: %s отсутствует: %w", p, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("проверка: файл %s пуст", p)
		}
		fmt.Printf("  Verified %s (%d bytes)\n", p, info.Size())
	}
	return nil
}

func (d *Deployer) dryRun(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	fmt.Println("[DRY RUN] Would perform the following:")
	fmt.Printf("  Write proxy.txt to %s (host=%s port=%d)\n", install.AppDir, proxyInfo.Host, proxyInfo.Port)
	for _, dll := range d.Config.DLLFiles {
		data, ok := assets.DLLData[dll]
		if !ok {
			continue
		}
		dst := filepath.Join(install.AppDir, dll)
		fmt.Printf("  Write %s -> %s (%d bytes)\n", dll, dst, len(data))
	}
	return nil
}

func (d *Deployer) Uninstall(install *discord.DiscordInstall) error {
	if IsDiscordRunning(install.Channel) && !d.Force {
		return fmt.Errorf("Discord запущен — сначала закройте Discord")
	}

	files := append([]string{d.Config.ProxyFile}, d.Config.DLLFiles...)
	var errs []error
	for _, name := range files {
		p := filepath.Join(install.AppDir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("ошибка удаления %s: %w", p, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func IsInstalled(install *discord.DiscordInstall, cfg *config.Config) bool {
	p := filepath.Join(install.AppDir, cfg.ProxyFile)
	_, err := os.Stat(p)
	return err == nil
}

// writeFile writes data atomically: it writes to a temp file in the same
// directory, fsyncs it, then renames over the target so an interrupted write
// cannot corrupt the destination (e.g. a partially written DLL).
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".szx-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeded

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// On Windows os.Rename fails if the target exists; remove it first.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpName, path)
}
