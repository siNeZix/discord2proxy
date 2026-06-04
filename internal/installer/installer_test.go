package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdQuote(t *testing.T) {
	got := cmdQuote(`C:\Users\me\App Data\discord2proxy`)
	want := `"C:\Users\me\App Data\discord2proxy"`
	if got != want {
		t.Fatalf("cmdQuote() = %q, want %q", got, want)
	}

	got = cmdQuote(`C:\bad"name\app`)
	want = `"C:\bad""name\app"`
	if got != want {
		t.Fatalf("cmdQuote() with quote = %q, want %q", got, want)
	}
}

func TestCopyFileAtomicallyReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	dst := filepath.Join(dir, "dst.exe")

	if err := os.WriteFile(src, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFileAtomically(src, dst); err != nil {
		t.Fatalf("copyFileAtomically() error = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("dst content = %q, want %q", got, "new")
	}
}

func TestCopyFileAtomicallyCreatesNew(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	dst := filepath.Join(dir, "dst.exe")

	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFileAtomically(src, dst); err != nil {
		t.Fatalf("copyFileAtomically() error = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("dst content = %q, want %q", got, "payload")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".szx-inst-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
