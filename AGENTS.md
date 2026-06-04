# discord2proxy

Discord SOCKS5 proxy deployer for Windows. (Go module path remains `discord-szx`.) Embeds DLLs into Discord's app directory to force proxy routing through v2ray/nekobox.

## Инструкции для AI Агента (AI Agent Instructions)

- **Язык общения**: Всегда общайся с пользователем исключительно на русском языке. Ответы должны быть лаконичными, без лишней вежливости и вводных фраз, но технически точными и полными.
- **Соблюдение соглашений**: Строго следуй архитектуре проекта, структуре файлов и стилю кодирования. перед изменениями читай связанные файлы.

## Build

```sh
make          # builds CLI, GUI and Setup to build/
make clean    # removes build/
```

Requires Go 1.26+, Windows only.

## Project Structure

```
cmd/main.go              — CLI entrypoint (flag-based)
cmd/gui/main.go          — GUI entrypoint (Gio framework)
cmd/setup/main.go        — Lightweight Console Setup loader
internal/assets/         — go:embed DLLs (DWrite.dll, force-proxy.dll)
internal/config/         — default paths, ports, version, repo coordinates
internal/deploy/         — deploy/uninstall/verify DLLs + proxy.txt into Discord
internal/discord/        — find Discord via registry + filesystem
internal/gui/            — Gio UI (dark theme, Russian localization)
internal/installer/      — system installation, shortcuts (COM IShellLink), registry (HKCU)
internal/proxy/          — SOCKS5 detection via handshake on ports 2080/1080
internal/update/         — GitHub Releases self-update (minio/selfupdate)
third_party/gioui.org/   — vendored, patched Gio (see "Vendored Gio" below)
.github/workflows/       — ci.yml (vet/build/test), release.yml (tag → Release)
```

## Vendored Gio

`go.mod` has `replace gioui.org => ./third_party/gioui.org`. The vendored copy is
Gio v0.9.0 with local patches:
1. `app/os_windows.go`: the window is centered on
   its monitor's work area at first windowed `Configure` (before `ShowWindow`),
   eliminating the startup jump where the window briefly appeared at the OS default
   top-left corner. Look for the `placed` field on `window` and the centering block
   in `Configure`'s `Windowed` case.
2. `widget/material/checkable.go`: the hover indicator (ellipse background on
   checkboxes/radiobuttons) is dimmed and neutralized (changed from alpha-70 of
   the accent/red color to a soft grey-blue color with alpha-18) to avoid an overly
   distracting, "eye-straining" neon glow when mouse-overing checkboxes.

- The whole module is committed (not gitignored) so CI/release builds pick up the
  patches — `windows-latest` builds use the replace automatically.
- If bumping Gio: re-copy the upstream module, clear read-only attrs, re-apply the
  centering patch and the checkable hover patch, then `go mod tidy`.

## Executable icons

Each command embeds its icon via a committed `rsrc_windows*.syso` (CI does not
generate them). Icon sources live in `assets/` (`favicon-original.png` for
CLI/GUI, `favicon-installer.png` for setup).

- `cmd/setup` uses an intentionally light icon (`cmd/setup/winres/`: 48/32/16 PNG,
  no 256×256) to keep `d2p-setup.exe` well under the 3 MB limit. The syso is
  `cmd/setup/rsrc_windows_windows_amd64.syso` (~7 KB).
- Regenerate it with [`go-winres`](https://github.com/tc-hib/go-winres)
  (`go install github.com/tc-hib/go-winres@latest`):
  `go-winres make --arch amd64 --in winres/winres.json --out rsrc_windows`
  (run from `cmd/setup/`). The `RT_GROUP_ICON` MUST use numeric ID `#1` — the
  Win32 window in `winui_windows.go` loads it via `LoadImage(hInst, 1, IMAGE_ICON…)`.
- The setup window is drawn with raw Win32 (not Gio), so the icon must be set on
  the window class (`wc.hIcon`/`hIconSm`) or Windows shows its default in the
  caption bar. Gio (CLI/GUI) loads icon ID 1 on its own.

## Architecture

1. **Proxy detection**: TCP connect + SOCKS5 handshake on 127.0.0.1:2080 (nekobox) then :1080 (v2ray)
2. **Discord finder**: Windows registry (`HKCU\...\Uninstall`) + known `%LocalAppData%` paths, sorted by channel priority (stable > ptb > canary > dev)
3. **Deploy**: Writes `proxy.txt` (SOCKS5_PROXY_ADDRESS/PORT) + copies embedded DLLs into Discord's `app-x.x.x` directory
4. **Uninstall**: Removes proxy.txt + DLLs from Discord directory
5. **GUI**: Gio-based 420x320 window with status cards, install/uninstall buttons, Russian UI
6. **System Install**:
   - Portable GUI at startup asks whether to install.
   - If "Install" is clicked: copies itself to `%LocalAppData%\discord2proxy\`, creates Desktop/StartMenu shortcuts via COM `IShellLinkW`, registers in `HKCU\...\Uninstall` (without UAC requirements), and relaunches.
   - If installed copy is launched from outside target directory (e.g., from old portable path), it automatically relaunches from the installed directory.
    - `d2p-setup.exe` is a lightweight console tool that downloads latest GUI via HTTP (with console progress bar), installs it and launches.
7. **Self-update**: On startup the GUI queries `GET /repos/siNeZix/discord2proxy/releases/latest`, compares against compiled-in `config.Version`, and (when newer) shows a top-right "Обновить vX.Y.Z" button. Clicking downloads the `d2p.exe` asset and atomically replaces the running binary via `minio/selfupdate`, then relaunches.

## Releases

Releases are tag-driven. Pushing a `vX.Y.Z` tag triggers `.github/workflows/release.yml`:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow builds all three binaries on `windows-latest` with `-X discord-szx/internal/config.Version=<tag-without-v>`, then publishes a GitHub Release with `d2p-cli.exe`, `d2p.exe`, `d2p-setup.exe` + `d2p-setup-upx.exe` attached.

- Versioning: `config.Version` is stored without a leading `v` (e.g. `0.1.0`); `config.VersionTag()` adds it for display. Keep all surfaces (window/footer/update button/tag) at the same `vX.Y.Z`.
- The GUI asset MUST stay named `d2p.exe` — `internal/update.AssetName` matches on it.
- `ci.yml` runs `go vet`/`build`/`test` on push & PR to main for pre-release safety.

## Key Details

- DLLs are embedded via `go:embed` in `internal/assets/` — no external data/ directory needed
- GUI uses `-H=windowsgui` linker flag to hide console
- Proxy config format: `SOCKS5_PROXY_ADDRESS=127.0.0.1\nSOCKS5_PROXY_PORT=2080`
- Discord running check via `tasklist` before deploy/uninstall
- Display name "Прокси для Discord" (`config.DisplayName`) is the GUI title; `config.AppName` ("discord2proxy") stays for file/process names
