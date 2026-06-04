package deploy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

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
// running. It walks a Toolhelp32 process snapshot via native Win32 calls, so
// no child process (tasklist) is spawned. On a snapshot failure it fails
// closed (returns true) so callers don't deploy over a possibly-running,
// file-locking Discord.
func isProcessRunning(exeName string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		// Fail closed: assume running rather than risk locked-file writes.
		return true
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) {
			return true
		}
	}
	return false
}

// IsDiscordRunning reports whether the Discord process for the given channel
// is currently running. Exposed for the GUI to poll without performing any
// deploy action.
func IsDiscordRunning(channel string) bool {
	return isProcessRunning(channelExeName(channel))
}

// killProcess force-terminates every process with the given image name via
// native Win32 calls (OpenProcess + TerminateProcess), so no child process
// (taskkill) is spawned. Discord's helper processes share the same image
// name, so terminating all matches covers the whole tree. Best-effort:
// individual open/terminate failures are ignored.
func killProcess(exeName string) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) {
			continue
		}
		h, oerr := windows.OpenProcess(windows.PROCESS_TERMINATE, false, e.ProcessID)
		if oerr != nil {
			continue
		}
		_ = windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
	return nil
}

// forceCloseDiscord terminates Discord for the channel and waits until the
// process has actually exited (and released its file locks), so subsequent
// writes to the app directory don't fail. Times out after ~5s.
func forceCloseDiscord(channel string) error {
	exe := channelExeName(channel)
	if err := killProcess(exe); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessRunning(exe) {
			// Give the OS a brief moment to release file handles.
			time.Sleep(300 * time.Millisecond)
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("не удалось завершить Discord (%s)", exe)
}

// startDiscord relaunches Discord for the channel as a detached process and
// returns without waiting for it to exit. It prefers the canonical launcher
// (Update.exe --processStart <exe>) in the install root, falling back to
// running the channel executable in the app directory directly. Best-effort:
// a missing launcher/exe surfaces as an error the caller may choose to ignore.
func startDiscord(install *discord.DiscordInstall) error {
	exe := channelExeName(install.Channel)

	updatePath := filepath.Join(install.Path, "Update.exe")
	if _, err := os.Stat(updatePath); err == nil {
		cmd := exec.Command(updatePath, "--processStart", exe)
		cmd.Dir = install.Path
		return cmd.Start()
	}

	// Fallback: launch the channel executable directly from the app directory.
	exePath := filepath.Join(install.AppDir, exe)
	cmd := exec.Command(exePath)
	cmd.Dir = install.AppDir
	return cmd.Start()
}

func formatProxyConfig(host string, port int) []byte {
	return []byte(fmt.Sprintf("SOCKS5_PROXY_ADDRESS=%s\nSOCKS5_PROXY_PORT=%d\n", host, port))
}

func (d *Deployer) Deploy(install *discord.DiscordInstall, proxyInfo *proxy.ProxyInfo) error {
	// Track whether Discord was force-closed so it can be relaunched after a
	// successful deploy (the force checkbox promises a restart).
	restart := false
	if IsDiscordRunning(install.Channel) {
		if !d.Force {
			return fmt.Errorf("Discord запущен — файлы заблокированы. Закройте Discord и попробуйте снова")
		}
		if err := forceCloseDiscord(install.Channel); err != nil {
			return err
		}
		restart = true
	}

	if d.DryRun {
		return d.dryRun(install, proxyInfo)
	}

	// Clear any leftovers from a prior uninstall before laying down fresh
	// files so the app directory stays clean.
	sweepLeftovers(install.AppDir)

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

	if restart {
		if err := startDiscord(install); err != nil {
			fmt.Printf("  Не удалось перезапустить Discord: %v\n", err)
		}
	}

	return nil
}

func (d *Deployer) Verify(install *discord.DiscordInstall) error {
	// Same order as Deploy/Uninstall: proxy.txt first, then DLLs.
	files := append([]string{d.Config.ProxyFile}, d.Config.DLLFiles...)
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
	// Track whether Discord was force-closed so it can be relaunched after a
	// successful uninstall (the force checkbox promises a restart).
	restart := false
	if IsDiscordRunning(install.Channel) {
		if !d.Force {
			return fmt.Errorf("Discord запущен — сначала закройте Discord")
		}
		if err := forceCloseDiscord(install.Channel); err != nil {
			return err
		}
		restart = true
	}

	files := append([]string{d.Config.ProxyFile}, d.Config.DLLFiles...)
	var errs []error
	for _, name := range files {
		p := filepath.Join(install.AppDir, name)
		if err := removeFile(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("ошибка удаления %s: %w", p, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	// Drop any leftovers from earlier deferred deletes so they don't linger.
	sweepLeftovers(install.AppDir)

	if restart {
		if err := startDiscord(install); err != nil {
			fmt.Printf("  Не удалось перезапустить Discord: %v\n", err)
		}
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
	// Atomically replace the target. os.Rename fails on Windows if the target
	// exists, and a Remove+Rename leaves a window where the file is missing
	// (Discord could read a half-applied state). MoveFileEx with
	// MOVEFILE_REPLACE_EXISTING swaps the file in a single atomic operation.
	from, err := windows.UTF16PtrFromString(tmpName)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// removeFile deletes path without the multi-second stall a plain os.Remove can
// incur on Windows when the target is a loaded/mapped DLL (DWrite.dll,
// force-proxy.dll): the kernel unlink of a mapped image goes through the
// filesystem filter stack (Defender real-time scan) synchronously.
//
// To stay cheap and non-blocking, it first renames the file aside (an atomic
// MoveFileEx that does not require the unlink to complete immediately), then
// removes the renamed copy. If even the rename is impossible because the file
// is still in use, it schedules deletion on the next reboot so the uninstall
// still completes and never hangs. A missing file is reported via the usual
// os.IsNotExist-compatible error so callers can ignore it.
func removeFile(path string) error {
	// Fast path: if the file isn't there, surface a NotExist error so the
	// caller's os.IsNotExist guard can skip it.
	if _, err := os.Lstat(path); err != nil {
		return err
	}

	// Plain remove first — succeeds instantly when nothing has the file open.
	if err := os.Remove(path); err == nil {
		return nil
	}

	// Move the file aside, then delete the (now unreferenced by name) copy.
	// MoveFileEx swaps the directory entry without waiting on the mapped
	// image, so it returns immediately even while Discord has the DLL loaded.
	// The aside name is unique per call so it never collides with a leftover
	// from a prior uninstall still pending a reboot delete (which would make
	// the rename fail even with MOVEFILE_REPLACE_EXISTING).
	aside := fmt.Sprintf("%s.%d%s", path, time.Now().UnixNano(), leftoverSuffix)
	from, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(aside)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING); err == nil {
		// Best-effort delete of the renamed file; if it's still mapped, defer
		// its removal to the next reboot. Either way the target name is gone.
		if rerr := os.Remove(aside); rerr != nil {
			scheduleDeleteOnReboot(aside)
		}
		return nil
	}

	// Couldn't even rename it: schedule the original for deletion on reboot so
	// the operation still finishes without blocking the UI.
	scheduleDeleteOnReboot(path)
	return nil
}

// scheduleDeleteOnReboot marks path for removal during the next system boot via
// MoveFileEx with MOVEFILE_DELAY_UNTIL_REBOOT (nil destination). Best-effort.
func scheduleDeleteOnReboot(path string) {
	from, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	_ = windows.MoveFileEx(from, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

// leftoverSuffix marks files renamed aside by removeFile when a plain delete
// could not complete because the target was still mapped.
const leftoverSuffix = ".szx-del"

// sweepLeftovers removes any *.szx-del files orphaned in dir by a previous
// removeFile call whose deferred delete never ran (e.g. no reboot happened
// since). Best-effort: still-locked leftovers are re-scheduled for the next
// reboot so they cannot accumulate in Discord's app directory.
func sweepLeftovers(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "*"+leftoverSuffix))
	if err != nil {
		return
	}
	for _, p := range matches {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			scheduleDeleteOnReboot(p)
		}
	}
}
