package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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

func applyProxyTemplate(template []byte, host string, port int) []byte {
	var lines []string
	for _, line := range strings.Split(string(template), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			switch strings.TrimSpace(kv[0]) {
			case "SOCKS5_PROXY_ADDRESS":
				line = "SOCKS5_PROXY_ADDRESS=" + host
			case "SOCKS5_PROXY_PORT":
				line = "SOCKS5_PROXY_PORT=" + strconv.Itoa(port)
			}
		}
		lines = append(lines, line)
	}
	return []byte(strings.Join(lines, "\n"))
}

func (d *Deployer) Deploy(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	if isDiscordRunning() && !d.Force {
		return fmt.Errorf("Discord is running — files may be locked. Close Discord or use --force")
	}

	if d.DryRun {
		return d.dryRun(install, proxyInfo)
	}

	templatePath := d.Config.ProxyFilePath()
	template, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read proxy template %s: %w", templatePath, err)
	}

	content := applyProxyTemplate(template, proxyInfo.Host, proxyInfo.Port)

	targetProxyPath := filepath.Join(install.AppDir, d.Config.ProxyFile)
	if err := writeFile(targetProxyPath, content); err != nil {
		return fmt.Errorf("write proxy.txt to %s: %w", targetProxyPath, err)
	}
	fmt.Printf("  Written %s\n", targetProxyPath)

	for _, dll := range d.Config.DLLFiles {
		src := d.Config.DLLSourcePath(dll)
		dst := filepath.Join(install.AppDir, dll)

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
		}
		fmt.Printf("  Copied %s -> %s\n", src, dst)
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
		src := d.Config.DLLSourcePath(dll)
		dst := filepath.Join(install.AppDir, dll)
		fmt.Printf("  Copy %s -> %s\n", src, dst)
	}
	return nil
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

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}
