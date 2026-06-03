package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

// InstallFrom copies the specified executable source to the installation directory, registers in Uninstall, and creates shortcuts.
func InstallFrom(srcExe string) error {
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

	// Create shortcuts in Desktop and Start Menu
	if err := createShortcuts(); err != nil {
		return fmt.Errorf("failed to create shortcuts: %w", err)
	}

	return nil
}

// InstallSelf copies the currently running executable to the installation directory, registers in Uninstall, and creates shortcuts.
func InstallSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}
	return InstallFrom(exe)
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
	// self-deletion. The running exe lives inside InstallDir, so the recursive
	// rmdir below removes it together with the rest of the install directory.
	//
	// Launch a detached cmd that waits briefly (so the current process can
	// exit and release its file lock) and then removes the whole install
	// directory recursively. rmdir alone deletes the running exe as well, so a
	// separate `del` is unnecessary. Commands are chained with `&` (not `&&`)
	// so the rmdir still runs even if the wait command reports a non-zero code.
	//
	// We set Dir to a safe location (like Temp or C:\) so that the spawned cmd
	// process doesn't inherit the installation directory as its working directory,
	// which would otherwise lock the folder and prevent rmdir from deleting it.
	cmd := exec.Command("cmd", "/c", "timeout /t 2 /nobreak >nul & rmdir /q /s "+cmdQuote(InstallDir()))
	cmd.Dir = os.Getenv("TEMP")
	if cmd.Dir == "" {
		cmd.Dir = "C:\\"
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	// Use empty/nil files to ensure the cmd process is fully detached
	// and doesn't hold standard output stream handles of the parent process.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start self-deletion command: %w", err)
	}

	return nil
}

// cmdQuote wraps a path in double quotes for use as a cmd.exe argument. cmd.exe
// does not support backslash-escaping of quotes; embedded quotes are doubled.
func cmdQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func copyFileAtomically(src, dst string) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".szx-inst-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

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

	// On Windows, remove target before rename
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpName, dst)
}
