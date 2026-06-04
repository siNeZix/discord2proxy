package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

var hideWindow = &syscall.SysProcAttr{HideWindow: true}

// InstallDir returns the target installation directory (%LocalAppData%\discord2proxy).
func InstallDir() string {
	localAppData := os.Getenv("LocalAppData")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(localAppData, "discord2proxy")
}

// InstalledExePath returns the path to the installed GUI binary inside the install directory.
func InstalledExePath() string {
	return filepath.Join(InstallDir(), "d2p.exe")
}

// IsInstalled checks if the application is installed in the system by verifying the registry key and the executable.
func IsInstalled() bool {
	_, err := os.Stat(InstalledExePath())
	if err != nil {
		return false
	}
	return isRegistryInstalled()
}

// IsRunningFromInstallDir reports whether the current process is running from the installed directory.
func IsRunningFromInstallDir() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}
	target, err := filepath.EvalSymlinks(InstalledExePath())
	if err != nil {
		target = InstalledExePath()
	}
	return strings.EqualFold(filepath.Clean(exe), filepath.Clean(target))
}

// InstallFrom copies the specified executable source to the installation
// directory, registers in Uninstall, and creates the requested shortcuts.
// desktop/startMenu select which shortcuts to create.
func InstallFrom(srcExe string, desktop, startMenu bool) error {
	dstExe := InstalledExePath()
	dstDir := InstallDir()

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Copy binary atomically
	if err := copyFileAtomically(srcExe, dstExe); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Write registry uninstall info
	if err := writeRegistry(); err != nil {
		return fmt.Errorf("failed to register uninstaller: %w", err)
	}

	// Create the requested shortcuts (Desktop and/or Start Menu)
	if err := createShortcuts(desktop, startMenu); err != nil {
		return fmt.Errorf("failed to create shortcuts: %w", err)
	}

	return nil
}

// InstallSelf copies the currently running executable to the installation
// directory, registers in Uninstall, and creates the requested shortcuts.
func InstallSelf(desktop, startMenu bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}
	return InstallFrom(exe, desktop, startMenu)
}

// RelaunchInstalled starts the application from the installed directory with the
// --no-relaunch flag. On success the new process is detached and this function
// returns nil; the caller is responsible for terminating the current process.
func RelaunchInstalled() error {
	exe := InstalledExePath()
	// Pass nil files or empty slice so that the new process is completely detached
	// and doesn't inherit open handles (like stdout/stderr of the calling console/process)
	// which blocks the parent from exiting/releasing resource locks.
	proc, err := os.StartProcess(exe, []string{exe, "--no-relaunch"}, &os.ProcAttr{
		Dir:   InstallDir(),
		Env:   os.Environ(),
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		return err
	}
	return proc.Release()
}

// Uninstall removes registry keys, shortcuts, and schedules the installed executable/directory for deletion.
func Uninstall() error {
	// Remove shortcuts
	removeShortcuts()

	// Remove registry key
	removeRegistry()

	// Since we are running (possibly as the installed exe itself), we schedule
	// self-deletion. The running exe lives inside InstallDir, so it holds an
	// NTFS lock on its own image: `rmdir /s` cannot delete a running exe and
	// therefore aborts, leaving the whole install directory behind. Relying on
	// a delay (so the process exits first) is racy and proved unreliable.
	//
	// NTFS forbids deleting a running exe but permits moving it. We move the
	// running image OUT of the install directory (into %TEMP%) so the install
	// directory is left holding only deletable files. The recursive rmdir can
	// then remove the whole directory at once, without having to wait for the
	// OS to release the image lock (which can take an unpredictably long time
	// after process exit — relying on rmdir retries against the still-locked
	// image proved unreliable). The orphaned image in %TEMP% is scheduled for
	// deletion on reboot and also retried by the cmd below.
	orphan := moveRunningExeOut()

	safeDir := os.Getenv("TEMP")
	if safeDir == "" {
		safeDir = os.Getenv("TMP")
	}
	if safeDir == "" {
		safeDir = `C:\`
	}

	// The deletion runs from a generated .bat file rather than a `cmd /c "..."`
	// one-liner. A one-liner suffers from nested-quoting breakage: the inner
	// quotes around each path collide with the outer quotes cmd uses to wrap the
	// whole `/c` argument, which silently broke `cd /d "<dir>"` (it never changed
	// directory, so the cmd's working directory stayed the install dir and rmdir
	// could not remove it). A batch file has no such nesting problem.
	if err := startDeleteBat(safeDir, InstallDir(), orphan); err != nil {
		return fmt.Errorf("failed to start self-deletion command: %w", err)
	}
	return nil
}

// startDeleteBat writes a self-deleting batch script to a temp file and launches
// it detached. The script:
//   - changes its working directory to safeDir (so it never locks installDir),
//   - retries `rmdir /s` on installDir until it is gone (the parent process needs
//     a moment to exit and release any lock),
//   - best-effort deletes the orphaned image left in %TEMP% (locked until the
//     parent exits and the OS releases the image handle),
//   - finally deletes itself.
//
// A batch file avoids the nested-quoting pitfalls of `cmd /c "..."` one-liners.
func startDeleteBat(safeDir, installDir, orphan string) error {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("cd /d " + cmdQuote(safeDir) + " 2>nul\r\n")
	// Give the parent process time to exit and release its locks.
	b.WriteString("ping 127.0.0.1 -n 3 >nul\r\n")
	// Retry removing the install directory until it is gone (or attempts run out).
	b.WriteString("set /a tries=0\r\n")
	b.WriteString(":rmloop\r\n")
	b.WriteString("rmdir /q /s " + cmdQuote(installDir) + " 2>nul\r\n")
	b.WriteString("if not exist " + cmdQuote(installDir) + " goto rmdone\r\n")
	b.WriteString("set /a tries+=1\r\n")
	b.WriteString("if %tries% geq 30 goto rmdone\r\n")
	b.WriteString("ping 127.0.0.1 -n 3 >nul\r\n")
	b.WriteString("goto rmloop\r\n")
	b.WriteString(":rmdone\r\n")
	if orphan != "" {
		// Retry deleting the orphan image (locked until the parent exits).
		b.WriteString("set /a otries=0\r\n")
		b.WriteString(":orloop\r\n")
		b.WriteString("del /q /f " + cmdQuote(orphan) + " 2>nul\r\n")
		b.WriteString("if not exist " + cmdQuote(orphan) + " goto ordone\r\n")
		b.WriteString("set /a otries+=1\r\n")
		b.WriteString("if %otries% geq 30 goto ordone\r\n")
		b.WriteString("ping 127.0.0.1 -n 3 >nul\r\n")
		b.WriteString("goto orloop\r\n")
		b.WriteString(":ordone\r\n")
	}
	// Self-delete the batch file. `del "%~f0"` removes the running script; cmd
	// keeps executing from memory, so it is safe.
	b.WriteString("del /q /f \"%~f0\"\r\n")

	batPath := filepath.Join(safeDir, fmt.Sprintf("d2p-uninstall-%d.bat", os.Getpid()))
	if err := os.WriteFile(batPath, []byte(b.String()), 0644); err != nil {
		return err
	}

	cmd := exec.Command("cmd", "/c", batPath)
	cmd.Dir = safeDir
	// CREATE_NO_WINDOW (0x08000000) gives the cmd its own session without
	// inheriting the parent's console AND without creating a visible console
	// window — so the `ping`-based delays don't flash a black console box.
	// We use it instead of DETACHED_PROCESS (0x8): DETACHED_PROCESS detaches the
	// console too, but forces the spawned cmd to allocate a brand-new VISIBLE
	// console (HideWindow is ignored once detached), which is exactly the window
	// the user saw. CREATE_NEW_PROCESS_GROUP keeps the cmd out of our process
	// group; together with Process.Release() below it survives the parent's
	// imminent exit so the deletion still runs.
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | syscall.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release the process handle so the child is fully detached and survives
	// the imminent exit of this process.
	_ = cmd.Process.Release()
	return nil
}

// moveRunningExeOut moves the currently running executable OUT of the install
// directory (into %TEMP%), freeing the install directory entirely. NTFS forbids
// deleting a running exe but permits renaming/moving it, so after this move the
// install directory contains only deletable files and the subsequent rmdir can
// remove the whole directory immediately — it no longer has to wait for the OS
// to release the image lock.
//
// The moved image cannot be deleted while the process lives, but because it now
// sits in %TEMP% (outside the install directory) it no longer blocks removal of
// the install directory. We schedule the orphan for deletion on the next reboot
// via MOVEFILE_DELAY_UNTIL_REBOOT, and also return its path so the caller can
// best-effort delete it after the process exits.
//
// Returns the orphan path (empty if no move happened, e.g. when not running
// from the install directory).
func moveRunningExeOut() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// Only relevant when running from inside the install directory; if launched
	// from elsewhere there is no self-lock to work around.
	if !strings.EqualFold(filepath.Dir(filepath.Clean(exe)), filepath.Clean(InstallDir())) {
		return ""
	}

	tmpDir := os.Getenv("TEMP")
	if tmpDir == "" {
		tmpDir = os.Getenv("TMP")
	}
	if tmpDir == "" {
		// Fall back to renaming within the same dir; the rmdir loop will then
		// have to wait for the OS to release the lock, but it's the best we can do.
		tmpDir = filepath.Dir(exe)
	}
	orphan := filepath.Join(tmpDir, fmt.Sprintf("d2p-old-%d.tmp", os.Getpid()))

	from, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	to, err := windows.UTF16PtrFromString(orphan)
	if err != nil {
		return ""
	}
	// MOVEFILE_COPY_ALLOWED permits crossing volumes if %TEMP% is on a different
	// drive than %LocalAppData% (rare, but possible).
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_COPY_ALLOWED); err != nil {
		// Could not move (e.g. cross-volume copy of a locked image). Leave the
		// exe where it is and rely on the rmdir retry loop after process exit.
		return ""
	}

	// Schedule the orphan for deletion on reboot as a guaranteed backstop.
	if orphanPtr, err := windows.UTF16PtrFromString(orphan); err == nil {
		_ = windows.MoveFileEx(orphanPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	return orphan
}

// cmdQuote wraps a path in double quotes for use as a cmd.exe argument. cmd.exe
// does not support backslash-escaping of quotes; embedded quotes are doubled.
//
// A trailing backslash is stripped first: a path ending in `\` would otherwise
// produce `...\"`, where cmd.exe treats `\"` as an escaped (literal) quote,
// breaking the argument's quoting (e.g. `cd /d "C:\...\Temp\"` silently fails to
// change directory). Drive roots like `C:\` are normalised to `C:` rather than
// emptied. Trailing backslashes are not meaningful for the directory/file paths
// we quote here, so dropping them is safe.
func cmdQuote(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\\' {
		s = s[:len(s)-1]
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func copyFileAtomically(src, dst string) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".szx-inst-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	moved := false
	defer func() {
		if !moved {
			os.Remove(tmpName)
		}
	}()

	in, err := os.Open(src)
	if err != nil {
		tmp.Close()
		return err
	}
	defer in.Close()

	if _, err := io.Copy(tmp, in); err != nil {
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

	from, err := windows.UTF16PtrFromString(tmpName)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	moved = true
	return nil
}
