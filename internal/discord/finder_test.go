package discord

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLatestAppDirUsesChannelExe(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		exeName string
	}{
		{name: "stable", dirName: "Discord", exeName: "Discord.exe"},
		{name: "ptb", dirName: "DiscordPTB", exeName: "DiscordPTB.exe"},
		{name: "canary", dirName: "DiscordCanary", exeName: "DiscordCanary.exe"},
		{name: "development", dirName: "DiscordDevelopment", exeName: "DiscordDevelopment.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tt.dirName)
			oldApp := filepath.Join(root, "app-1.0.1")
			newApp := filepath.Join(root, "app-1.0.2")
			if err := os.MkdirAll(oldApp, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(newApp, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(oldApp, tt.exeName), []byte{}, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(newApp, tt.exeName), []byte{}, 0644); err != nil {
				t.Fatal(err)
			}

			gotDir, gotVersion, ok := findLatestAppDir(root)
			if !ok {
				t.Fatal("findLatestAppDir() ok = false")
			}
			if gotDir != newApp {
				t.Fatalf("dir = %q, want %q", gotDir, newApp)
			}
			if gotVersion != "1.0.2" {
				t.Fatalf("version = %q, want %q", gotVersion, "1.0.2")
			}
		})
	}
}
