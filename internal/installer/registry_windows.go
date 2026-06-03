package installer

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/windows/registry"

	"discord-szx/internal/config"
)

const registryKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\discord2proxy`

func isRegistryInstalled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func writeRegistry() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, registryKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	exePath := InstalledExePath()
	instDir := InstallDir()

	sizeKB := uint32(0)
	if info, err := os.Stat(exePath); err == nil {
		sizeKB = uint32(info.Size() / 1024)
	}

	// DisplayName and UninstallString are required for the entry to be usable
	// from "Apps & features"; failing to write them means a broken install, so
	// surface those errors. The rest are cosmetic and best-effort.
	if err := k.SetStringValue("DisplayName", config.DisplayName); err != nil {
		return fmt.Errorf("set DisplayName: %w", err)
	}
	if err := k.SetStringValue("UninstallString", `"`+exePath+`" --uninstall`); err != nil {
		return fmt.Errorf("set UninstallString: %w", err)
	}
	_ = k.SetStringValue("DisplayVersion", config.Version)
	_ = k.SetStringValue("Publisher", "siNeZix")
	_ = k.SetStringValue("DisplayIcon", exePath)
	_ = k.SetStringValue("InstallLocation", instDir)
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	if sizeKB > 0 {
		_ = k.SetDWordValue("EstimatedSize", sizeKB)
	}

	return nil
}

func removeRegistry() {
	err := registry.DeleteKey(registry.CURRENT_USER, registryKeyPath)
	if err != nil && err != registry.ErrNotExist {
		log.Printf("installer: failed to delete registry key: %v", err)
	}
}
