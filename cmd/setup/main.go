//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"discord-szx/internal/config"
	"discord-szx/internal/installer"
	"discord-szx/internal/version"
)

var (
	wininet                 = syscall.NewLazyDLL("wininet.dll")
	procInternetOpen        = wininet.NewProc("InternetOpenW")
	procInternetOpenUrl     = wininet.NewProc("InternetOpenUrlW")
	procInternetReadFile    = wininet.NewProc("InternetReadFile")
	procInternetCloseHandle = wininet.NewProc("InternetCloseHandle")
	procHttpQueryInfo       = wininet.NewProc("HttpQueryInfoW")
)

// wininet INTERNET_FLAG_* combined for a fresh, uncached, HTTPS-capable request.
const internetFlags = uintptr(0x80000000 | 0x04000000 | 0x00800000) // RELOAD | DONT_CACHE | SECURE

// progressFunc reports download progress. total is -1 when the size is unknown.
type progressFunc func(downloaded, total int64)

// downloadWininet downloads a URL to a local filepath using wininet.dll. It
// reports progress through the optional callback (may be nil).
func downloadWininet(url string, dstPath string, onProgress progressFunc) error {
	hInternet, err := internetOpen()
	if err != nil {
		return err
	}
	defer procInternetCloseHandle.Call(hInternet)

	hUrl, err := internetOpenURL(hInternet, url)
	if err != nil {
		return err
	}
	defer procInternetCloseHandle.Call(hUrl)

	// Query Content-Length to drive a percentage progress bar. With
	// HTTP_QUERY_FLAG_NUMBER the value is written back as a 32-bit DWORD.
	var contentLength int64 = -1
	var dword uint32
	dwordLen := uint32(unsafe.Sizeof(dword))
	// HTTP_QUERY_CONTENT_LENGTH = 5, HTTP_QUERY_FLAG_NUMBER = 0x20000000
	queryFlag := uintptr(5 | 0x20000000)
	var index uint32
	ok, _, _ := procHttpQueryInfo.Call(
		hUrl,
		queryFlag,
		uintptr(unsafe.Pointer(&dword)),
		uintptr(unsafe.Pointer(&dwordLen)),
		uintptr(unsafe.Pointer(&index)),
	)
	if ok != 0 {
		contentLength = int64(dword)
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var downloaded int64
	var lastUpdate time.Time
	if onProgress != nil {
		onProgress(0, contentLength)
	}

	readBuf := make([]byte, 32*1024)
	for {
		var bytesRead uint32
		r, _, callErr := procInternetReadFile.Call(
			hUrl,
			uintptr(unsafe.Pointer(&readBuf[0])),
			uintptr(len(readBuf)),
			uintptr(unsafe.Pointer(&bytesRead)),
		)
		if r == 0 {
			return fmt.Errorf("InternetReadFile failed: %v", callErr)
		}
		if bytesRead == 0 {
			break
		}

		if _, err := out.Write(readBuf[:bytesRead]); err != nil {
			return err
		}

		downloaded += int64(bytesRead)
		if onProgress != nil {
			if time.Since(lastUpdate) > 80*time.Millisecond || (contentLength > 0 && downloaded == contentLength) {
				lastUpdate = time.Now()
				onProgress(downloaded, contentLength)
			}
		}
	}

	if onProgress != nil {
		onProgress(downloaded, contentLength)
	}
	return nil
}

// internetOpen initializes a wininet session with a direct (non-proxied)
// connection. The caller must close the returned handle.
func internetOpen() (uintptr, error) {
	agentPtr, _ := syscall.UTF16PtrFromString(config.AppName)
	// InternetOpenW(lpszAgent, dwAccessType=INTERNET_OPEN_TYPE_PRECONFIG, ...)
	// 0 = PRECONFIG: honor the system/IE proxy settings (corp networks).
	h, _, callErr := procInternetOpen.Call(
		uintptr(unsafe.Pointer(agentPtr)),
		0,
		0, 0, 0,
	)
	if h == 0 {
		return 0, fmt.Errorf("InternetOpen failed: %v", callErr)
	}
	return h, nil
}

// internetOpenURL opens an HTTP(S) URL on an existing session. The caller must
// close the returned handle.
func internetOpenURL(hInternet uintptr, url string) (uintptr, error) {
	urlPtr, _ := syscall.UTF16PtrFromString(url)
	h, _, callErr := procInternetOpenUrl.Call(
		hInternet,
		uintptr(unsafe.Pointer(urlPtr)),
		0, 0, internetFlags, 0,
	)
	if h == 0 {
		return 0, fmt.Errorf("InternetOpenUrl failed: %v", callErr)
	}
	return h, nil
}

// getLatestAssetURL builds the stable redirect URL for the latest GUI asset.
func getLatestAssetURL() string {
	// В релизах на гитхабе GUI-бинарник называется discord2proxy-gui.exe
	return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/discord2proxy-gui.exe", config.RepoOwner, config.RepoName)
}

// getLatestVersionTagAPI fetches the latest release tag via the GitHub API,
// parsing only the "tag_name" field to avoid pulling in encoding/json.
func getLatestVersionTagAPI() (string, error) {
	hInternet, err := internetOpen()
	if err != nil {
		return "", err
	}
	defer procInternetCloseHandle.Call(hInternet)

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", config.RepoOwner, config.RepoName)
	hUrl, err := internetOpenURL(hInternet, apiURL)
	if err != nil {
		return "", err
	}
	defer procInternetCloseHandle.Call(hUrl)

	// Read up to 32KB of the JSON response. tag_name appears near the top.
	buf := make([]byte, 32*1024)
	var totalRead uint32
	for totalRead < uint32(len(buf)) {
		var bytesRead uint32
		r, _, _ := procInternetReadFile.Call(
			hUrl,
			uintptr(unsafe.Pointer(&buf[totalRead])),
			uintptr(len(buf)-int(totalRead)),
			uintptr(unsafe.Pointer(&bytesRead)),
		)
		if r == 0 || bytesRead == 0 {
			break
		}
		totalRead += bytesRead
	}

	response := string(buf[:totalRead])
	idx := strings.Index(response, `"tag_name":`)
	if idx == -1 {
		return "", fmt.Errorf("tag_name not found in release JSON")
	}
	sub := strings.TrimSpace(response[idx+len(`"tag_name":`):])
	if len(sub) == 0 || (sub[0] != '"' && sub[0] != '\'') {
		return "", fmt.Errorf("tag_name not quoted")
	}
	quoteChar := sub[0]
	endIdx := strings.IndexByte(sub[1:], quoteChar)
	if endIdx == -1 {
		return "", fmt.Errorf("tag_name closing quote not found")
	}
	return sub[1 : endIdx+1], nil
}

// alreadyUpToDate reports whether an up-to-date installation is already present
// in the system. When true, the caller should relaunch the installed copy
// instead of downloading anything.
func alreadyUpToDate() bool {
	if !installer.IsInstalled() {
		return false
	}
	installedVer := installer.GetInstalledVersion()
	if installedVer == "" {
		return false
	}

	latestTag, err := getLatestVersionTagAPI()
	if err != nil {
		// Offline / API unreachable: prefer launching the local copy over
		// failing the whole setup.
		return true
	}
	latestVer := strings.TrimPrefix(latestTag, "v")
	return !version.IsNewer(installedVer, latestVer)
}

// runInstall downloads the latest GUI build and installs it into the system,
// creating the requested shortcuts. Progress is reported via onProgress.
func runInstall(desktop, startMenu bool, onProgress progressFunc) error {
	tmpFile, err := os.CreateTemp("", "d2p-*.exe")
	if err != nil {
		return fmt.Errorf("создание временного файла: %w", err)
	}
	tempExe := tmpFile.Name()
	tmpFile.Close() // Close it so wininet can write to it safely.
	defer os.Remove(tempExe)

	if err := downloadWininet(getLatestAssetURL(), tempExe, onProgress); err != nil {
		return fmt.Errorf("скачивание: %w", err)
	}

	if err := installer.InstallFrom(tempExe, desktop, startMenu); err != nil {
		return fmt.Errorf("установка: %w", err)
	}
	return nil
}

func main() {
	// If an up-to-date copy is already installed, just relaunch it without
	// showing the installer window at all.
	if alreadyUpToDate() {
		if err := installer.RelaunchInstalled(); err == nil {
			os.Exit(0)
		}
		// Fall through to the window on relaunch failure.
	}

	runWindow()
}
