package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"discord-szx/internal/config"
	"discord-szx/internal/installer"
	"discord-szx/internal/update"
)

type progressWriter struct {
	total      int64
	downloaded int64
	lastUpdate time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)

	if time.Since(pw.lastUpdate) > 100*time.Millisecond || pw.downloaded == pw.total {
		pw.lastUpdate = time.Now()
		pw.drawProgress()
	}
	return n, nil
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

	fmt.Printf("\r  Скачивание: [%s] %.1f%% (%.2f/%.2f MB)   ", bar, pct*100, downloadedMB, totalMB)
	if pw.downloaded == pw.total {
		fmt.Println()
	}
}

func main() {
	fmt.Printf("=== %s: Установка ===\n", config.DisplayName)
	fmt.Println("Поиск последней версии...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rel, _, err := update.CheckLatest(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка проверки обновлений: %v\n", err)
		os.Exit(1)
	}
	if rel == nil || rel.AssetURL == "" {
		fmt.Fprintln(os.Stderr, "Ошибка: не удалось найти подходящий ассет для установки в релизах GitHub.")
		os.Exit(1)
	}

	fmt.Printf("Доступна версия: %s\n", rel.Tag)

	tmpFile, err := os.CreateTemp("", "discord2proxy-gui-*.exe")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка создания временного файла: %v\n", err)
		os.Exit(1)
	}
	tempExe := tmpFile.Name()
	defer os.Remove(tempExe)

	// Download file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.AssetURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка создания запроса: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", "discord2proxy-setup")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка скачивания: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Ошибка сервера при скачивании: %s\n", resp.Status)
		os.Exit(1)
	}

	pw := &progressWriter{
		total:      resp.ContentLength,
		lastUpdate: time.Now(),
	}

	// Stream the download into the temp file while feeding the progress writer.
	_, err = io.Copy(tmpFile, io.TeeReader(resp.Body, pw))
	if err != nil {
		tmpFile.Close()
		fmt.Fprintf(os.Stderr, "\nОшибка при сохранении файла: %v\n", err)
		os.Exit(1)
	}
	if err := tmpFile.Close(); err != nil { // close before using for install
		fmt.Fprintf(os.Stderr, "\nОшибка при сохранении файла: %v\n", err)
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
