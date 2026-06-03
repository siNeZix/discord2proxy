package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"discord-szx/internal/gui"
	"discord-szx/internal/installer"
)

func main() {
	installFlag := flag.Bool("install", false, "Install the application to the system")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall the application from the system")
	noRelaunchFlag := flag.Bool("no-relaunch", false, "Disable automatic relaunch from the installation directory")
	flag.Parse()

	if *installFlag {
		if err := installer.InstallSelf(true, true); err != nil {
			messageBox("Ошибка установки", fmt.Sprintf("Не удалось установить приложение: %v", err), 0x10) // MB_ICONERROR
			os.Exit(1)
		}
		if err := installer.RelaunchInstalled(); err != nil {
			messageBox("Ошибка перезапуска", fmt.Sprintf("Не удалось запустить установленную копию: %v", err), 0x10)
			os.Exit(1)
		}
		// RelaunchInstalled spawned the installed copy; exit so we don't run two instances.
		os.Exit(0)
	}

	if *uninstallFlag {
		if err := installer.Uninstall(); err != nil {
			messageBox("Ошибка удаления", fmt.Sprintf("Не удалось удалить приложение: %v", err), 0x10)
			os.Exit(1)
		}
		// Allow any scheduled background commands (like self-deletion CMD)
		// a tiny split second to ensure their handles are initiated before we exit
		os.Exit(0)
	}

	gui.Run(*noRelaunchFlag)
}

func messageBox(title, text string, style uint32) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBoxW := user32.NewProc("MessageBoxW")
	tPtr, _ := syscall.UTF16PtrFromString(title)
	txtPtr, _ := syscall.UTF16PtrFromString(text)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(txtPtr)), uintptr(unsafe.Pointer(tPtr)), uintptr(style))
}
