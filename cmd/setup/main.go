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

type progressWriter struct {
	total      int64
	downloaded int64
	lastUpdate time.Time
}

func (pw *progressWriter) drawProgress() {
	const barWidth = 30
	var pct float64
	if pw.total > 0 {
		pct = float64(pw.downloaded) / float64(pw.total)
	}
	filled := int(pct * barWidth)
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Format sizes as MB
	downloadedMB := float64(pw.downloaded) / (1024 * 1024)
	totalMB := float64(pw.total) / (1024 * 1024)

	if pw.total > 0 {
		fmt.Printf("\r  Скачивание: [%s] %.1f%% (%.2f/%.2f MB)   ", bar, pct*100, downloadedMB, totalMB)
	} else {
		fmt.Printf("\r  Скачивание: (%.2f MB)   ", downloadedMB)
	}
}

// downloadWininet downloads a URL to a local filepath using wininet.dll
func downloadWininet(url string, dstPath string) error {
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

	// Query Content-Length to show a percentage progress bar. With
	// HTTP_QUERY_FLAG_NUMBER the value is written back as a 32-bit DWORD.
	var contentLength int64
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

	pw := &progressWriter{
		total:      contentLength,
		lastUpdate: time.Now(),
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

		pw.downloaded += int64(bytesRead)
		if time.Since(pw.lastUpdate) > 100*time.Millisecond || (pw.total > 0 && pw.downloaded == pw.total) {
			pw.lastUpdate = time.Now()
			pw.drawProgress()
		}
	}

	// Always render the final state, then break the progress line.
	pw.drawProgress()
	fmt.Println()

	return nil
}

// internetOpen initializes a wininet session with a direct (non-proxied)
// connection. The caller must close the returned handle.
func internetOpen() (uintptr, error) {
	agentPtr, _ := syscall.UTF16PtrFromString("discord2proxy-setup")
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
	// Asset name must match internal/update.AssetName and the release workflow.
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

func main() {
	fmt.Printf("=== %s: Установка ===\n", config.DisplayName)

	// Check if already installed
	installedVer := installer.GetInstalledVersion()
	hasInstallation := installer.IsInstalled()

	if hasInstallation && installedVer != "" {
		fmt.Printf("Обнаружена установленная версия в системе: %s\n", installedVer)
		fmt.Println("Проверка наличия обновлений на сервере...")

		latestTag, err := getLatestVersionTagAPI()
		if err == nil {
			latestVer := strings.TrimPrefix(latestTag, "v")
			if !version.IsNewer(installedVer, latestVer) {
				fmt.Printf("У вас уже установлена последняя версия (%s). Перезапуск...\n", installedVer)
				time.Sleep(1 * time.Second)
				if err := installer.RelaunchInstalled(); err != nil {
					fmt.Fprintf(os.Stderr, "Не удалось запустить установленное приложение: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			} else {
				fmt.Printf("Доступна новая версия: %s (у вас: %s). Начинаем обновление...\n", latestVer, installedVer)
			}
		} else {
			// If we failed to get version online (e.g. offline), let's fallback to launching installed app!
			fmt.Printf("Не удалось связаться с сервером обновлений (%v). Запуск локальной версии...\n", err)
			time.Sleep(1 * time.Second)
			if err := installer.RelaunchInstalled(); err == nil {
				os.Exit(0)
			}
		}
	}

	fmt.Println("Запуск скачивания последней версии...")

	tmpFile, err := os.CreateTemp("", "discord2proxy-gui-*.exe")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка создания временного файла: %v\n", err)
		os.Exit(1)
	}
	tempExe := tmpFile.Name()
	tmpFile.Close() // Close it so wininet can overwrite/write to it safely
	defer os.Remove(tempExe)

	assetURL := getLatestAssetURL()

	err = downloadWininet(assetURL, tempExe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nОшибка при скачивании файла: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Выполнение установки в систему...")

	if err := installer.InstallFrom(tempExe); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка установки: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Установка завершена успешно! Запуск установленного приложения...")

	// Launch installed GUI
	if err := installer.RelaunchInstalled(); err != nil {
		fmt.Fprintf(os.Stderr, "Не удалось запустить установленное приложение: %v\n", err)
		os.Exit(1)
	}
}
