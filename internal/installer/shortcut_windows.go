package installer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"discord-szx/internal/config"
)

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

type clsid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type iid clsid

var (
	clsidShellLink = clsid{0x00021401, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW  = iid{0x000214F9, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = iid{0x0000010b, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type iUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type iShellLinkWVtbl struct {
	iUnknownVtbl
	GetPath             uintptr
	GetIDList           uintptr
	SetIDList           uintptr
	GetDescription      uintptr
	SetDescription      uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments        uintptr
	SetArguments        uintptr
	GetHotkey           uintptr
	SetHotkey           uintptr
	GetShowCmd          uintptr
	SetShowCmd          uintptr
	GetIconLocation     uintptr
	SetIconLocation     uintptr
	SetRelativePath     uintptr
	Resolve             uintptr
	SetPath             uintptr
}

type iPersistFileVtbl struct {
	iUnknownVtbl
	GetClassID    uintptr
	IsDirty       uintptr
	Load          uintptr
	Save          uintptr
	SaveCompleted uintptr
	GetCurFile    uintptr
}

type iShellLinkW struct {
	vtbl *iShellLinkWVtbl
}

type iPersistFile struct {
	vtbl *iPersistFileVtbl
}

func desktopLnkPath() string {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		return ""
	}
	return filepath.Join(userProfile, "Desktop", config.DisplayName+".lnk")
}

func startMenuLnkPath() string {
	appData := os.Getenv("AppData")
	if appData == "" {
		return ""
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", config.DisplayName+".lnk")
}

// createShortcuts writes the requested .lnk files. desktop/startMenu select
// which targets to create; if both are false it is a no-op.
func createShortcuts(desktop, startMenu bool) error {
	if !desktop && !startMenu {
		return nil
	}

	// Initialize COM. CoUninitialize must be balanced against a successful
	// CoInitializeEx only. RPC_E_CHANGED_MODE means COM was already initialized
	// on this thread with a different mode: we can still use it, but we must NOT
	// call CoUninitialize for an init we didn't perform.
	const rpcEChangedMode = 0x80010106
	res, _, _ := procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED = 2
	if int32(res) < 0 && uint32(res) != rpcEChangedMode {
		return fmt.Errorf("CoInitializeEx failed: 0x%x", res)
	}
	if uint32(res) != rpcEChangedMode {
		defer procCoUninitialize.Call()
	}

	var pSL *iShellLinkW
	res, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)),
		0,
		1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidShellLinkW)),
		uintptr(unsafe.Pointer(&pSL)),
	)
	if int32(res) < 0 {
		return fmt.Errorf("CoCreateInstance(ShellLink) failed: 0x%x", res)
	}
	defer syscall.SyscallN(pSL.vtbl.Release, uintptr(unsafe.Pointer(pSL)))

	// Set paths
	exePath := InstalledExePath()
	instDir := InstallDir()
	exePathUTF16, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	instDirUTF16, err := syscall.UTF16PtrFromString(instDir)
	if err != nil {
		return err
	}

	// IShellLinkW::SetPath
	syscall.SyscallN(pSL.vtbl.SetPath, uintptr(unsafe.Pointer(pSL)), uintptr(unsafe.Pointer(exePathUTF16)))
	// IShellLinkW::SetWorkingDirectory
	syscall.SyscallN(pSL.vtbl.SetWorkingDirectory, uintptr(unsafe.Pointer(pSL)), uintptr(unsafe.Pointer(instDirUTF16)))

	// Query IPersistFile
	var pPF *iPersistFile
	res, _, _ = syscall.SyscallN(
		pSL.vtbl.QueryInterface,
		uintptr(unsafe.Pointer(pSL)),
		uintptr(unsafe.Pointer(&iidPersistFile)),
		uintptr(unsafe.Pointer(&pPF)),
	)
	if int32(res) < 0 {
		return fmt.Errorf("QueryInterface(IPersistFile) failed: 0x%x", res)
	}
	defer syscall.SyscallN(pPF.vtbl.Release, uintptr(unsafe.Pointer(pPF)))

	var requested, saved int
	var lastErr error

	// Save Desktop shortcut
	if desktop {
		if path := desktopLnkPath(); path != "" {
			requested++
			if err := saveShortcut(pPF, path); err != nil {
				lastErr = err
			} else {
				saved++
			}
		}
	}

	// Save Start Menu shortcut
	if startMenu {
		if path := startMenuLnkPath(); path != "" {
			requested++
			_ = os.MkdirAll(filepath.Dir(path), 0755)
			if err := saveShortcut(pPF, path); err != nil {
				lastErr = err
			} else {
				saved++
			}
		}
	}

	// Tolerate a single failure (e.g. roaming Start Menu unavailable) as long
	// as at least one shortcut was written; fail only if none could be created.
	if requested > 0 && saved == 0 && lastErr != nil {
		return fmt.Errorf("failed to save any shortcut: %w", lastErr)
	}

	return nil
}

// saveShortcut writes the link target via IPersistFile::Save(pszFileName, fRemember=TRUE).
func saveShortcut(pPF *iPersistFile, path string) error {
	pathUTF16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	res, _, _ := syscall.SyscallN(pPF.vtbl.Save, uintptr(unsafe.Pointer(pPF)), uintptr(unsafe.Pointer(pathUTF16)), 1)
	if int32(res) < 0 {
		return fmt.Errorf("IPersistFile::Save(%q) failed: 0x%x", path, uint32(res))
	}
	return nil
}

func removeShortcuts() {
	if desktop := desktopLnkPath(); desktop != "" {
		err := os.Remove(desktop)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("installer: failed to delete desktop shortcut: %v", err)
		}
	}
	if startMenu := startMenuLnkPath(); startMenu != "" {
		err := os.Remove(startMenu)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("installer: failed to delete start menu shortcut: %v", err)
		}
	}
}
