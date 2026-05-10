package deploy

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"discord-szx/internal/assets"
	"discord-szx/internal/config"
	"discord-szx/internal/discord"
	"discord-szx/internal/proxy"
)

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

func isDiscordRunning() bool {
	out, err := exec.Command("tasklist", "/NH", "/FI", "IMAGENAME eq Discord.exe").Output()
	if err != nil {
		return false
	}
	return bytes.Contains(out, []byte("Discord.exe"))
}

func formatProxyConfig(host string, port int) []byte {
	return []byte(fmt.Sprintf("SOCKS5_PROXY_ADDRESS=%s\nSOCKS5_PROXY_PORT=%d\n", host, port))
}

var dllData = map[string][]byte{
	"DWrite.dll":      assets.DWriteDLL,
	"force-proxy.dll": assets.ForceProxyDLL,
}

func (d *Deployer) Deploy(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	if isDiscordRunning() && !d.Force {
		return fmt.Errorf("Discord is running — files may be locked. Close Discord or use --force")
	}

	if d.DryRun {
		return d.dryRun(install, proxyInfo)
	}

	content := formatProxyConfig(proxyInfo.Host, proxyInfo.Port)

	targetProxyPath := filepath.Join(install.AppDir, d.Config.ProxyFile)
	if err := writeFile(targetProxyPath, content); err != nil {
		return fmt.Errorf("write proxy.txt to %s: %w", targetProxyPath, err)
	}
	fmt.Printf("  Written %s\n", targetProxyPath)

	for _, dll := range d.Config.DLLFiles {
		data, ok := dllData[dll]
		if !ok {
			return fmt.Errorf("unknown asset %s", dll)
		}
		dst := filepath.Join(install.AppDir, dll)
		if err := writeFile(dst, data); err != nil {
			return fmt.Errorf("write %s to %s: %w", dll, dst, err)
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
			return fmt.Errorf("verify: %s missing: %w", p, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("verify: %s is empty", p)
		}
		fmt.Printf("  Verified %s (%d bytes)\n", p, info.Size())
	}
	return nil
}

func (d *Deployer) dryRun(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	fmt.Println("[DRY RUN] Would perform the following:")
	fmt.Printf("  Write proxy.txt to %s (host=%s port=%d)\n", install.AppDir, proxyInfo.Host, proxyInfo.Port)
	for _, dll := range d.Config.DLLFiles {
		data, ok := dllData[dll]
		if !ok {
			continue
		}
		dst := filepath.Join(install.AppDir, dll)
		fmt.Printf("  Write %s -> %s (%d bytes)\n", dll, dst, len(data))
	}
	return nil
}

func (d *Deployer) Uninstall(install *discord.DiscordInstall) error {
	if isDiscordRunning() && !d.Force {
		return fmt.Errorf("Discord is running — close Discord first")
	}

	files := append([]string{d.Config.ProxyFile}, d.Config.DLLFiles...)
	for _, name := range files {
		p := filepath.Join(install.AppDir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return nil
}

func IsInstalled(install *discord.DiscordInstall, cfg *config.Config) bool {
	p := filepath.Join(install.AppDir, cfg.ProxyFile)
	_, err := os.Stat(p)
	return err == nil
}

func writeFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
