package main

import (
	"discord-szx/internal/gui"
	"syscall"
)

func main() {
	// Detach from console if present to prevent flashing console window
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	freeConsole.Call()

	gui.Run()
}
