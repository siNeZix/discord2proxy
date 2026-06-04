# discord2proxy

> **🇷🇺 [Читать на русском → README.ru.md](README.ru.md)**

A SOCKS5 proxy deployer for **Discord** on **Windows**. It forces Discord's
network traffic through a local SOCKS5 proxy (v2ray / nekobox) by embedding a
small loader DLL and a `proxy.txt` config into Discord's application directory.

No system-wide proxy, no VPN, no admin rights required — only Discord is routed.

---

## How it works

1. **Proxy detection** — performs a TCP connect + SOCKS5 handshake on
   `127.0.0.1:2080` (nekobox), then `127.0.0.1:1080` (v2ray), and picks the
   first live endpoint.
2. **Discord finder** — locates Discord via the Windows registry
   (`HKCU\...\Uninstall`) and known `%LocalAppData%` paths, sorted by channel
   priority: `stable > ptb > canary > development`.
3. **Deploy** — writes `proxy.txt` (`SOCKS5_PROXY_ADDRESS` / `SOCKS5_PROXY_PORT`)
   and copies the embedded DLLs (`DWrite.dll`, `force-proxy.dll`) into Discord's
   `app-x.x.x` directory.
4. **Verify** — confirms the files landed correctly. Restart Discord to apply.
5. **Uninstall** — removes `proxy.txt` and the DLLs from the Discord directory.

The proxy config format written to `proxy.txt`:

```
SOCKS5_PROXY_ADDRESS=127.0.0.1
SOCKS5_PROXY_PORT=2080
```

---

## Components

| Binary           | Description                                                              |
| ---------------- | ------------------------------------------------------------------------ |
| `d2p.exe`        | GUI (Gio framework). Dark theme, Russian UI, install / uninstall cards.  |
| `d2p-cli.exe`    | Command-line deployer (flag-based).                                      |
| `d2p-setup.exe`  | Lightweight setup loader: downloads the latest GUI and installs it.      |

### GUI (`d2p.exe`)

- 420×320 window with status cards and install / uninstall buttons.
- Offers to install itself to `%LocalAppData%\discord2proxy\`, with Desktop and
  Start Menu shortcuts (via COM `IShellLinkW`) and an HKCU uninstall entry — no
  UAC prompt.
- **Self-update**: on startup it queries the latest GitHub Release, compares it
  against the compiled-in version, and shows an "Обновить vX.Y.Z" button when a
  newer build is available. Clicking atomically replaces the running binary and
  relaunches.

### CLI (`d2p-cli.exe`)

```
d2p-cli [flags]
```

| Flag             | Default     | Description                                                  |
| ---------------- | ----------- | ------------------------------------------------------------ |
| `-proxy-port`    | `0` (auto)  | Override proxy port (`0` = auto-detect on 2080 / 1080).      |
| `-proxy-host`    | `127.0.0.1` | Override proxy host.                                         |
| `-discord-path`  | _(empty)_   | Override the Discord installation path.                     |
| `-channel`       | _(empty)_   | Select channel: `stable` \| `ptb` \| `canary` \| `development` (alias `dev`). |
| `-dry-run`       | `false`     | Show what would be done without executing.                  |
| `-force`         | `false`     | Deploy even if Discord is running / proxy is unreachable.   |
| `-no-verify`     | `false`     | Skip post-deploy verification.                              |

### Setup (`d2p-setup.exe`)

A native Win32 (non-Gio) console/window loader that downloads the latest
`d2p.exe` over HTTP (with a progress indicator), installs it, and launches it.
A UPX-compressed variant (`d2p-setup-upx.exe`) is also published.

---

## Build

Requires **Go 1.26+**, **Windows only**.

```sh
make          # builds CLI, GUI and Setup into build/
make clean    # removes build/
```

Individual targets: `make build`, `make build-gui`, `make build-setup`,
`make build-setup-upx`.

The version is injected at build time from the git tag via:

```
-ldflags "-X discord-szx/internal/config.Version=<version>"
```

---

## Releases

Releases are tag-driven. Pushing a `vX.Y.Z` tag triggers the release workflow,
which builds all binaries on `windows-latest` and publishes a GitHub Release
with `d2p-cli.exe`, `d2p.exe`, `d2p-setup.exe` and `d2p-setup-upx.exe` attached.

```sh
git tag v0.5.0
git push origin v0.5.0
```

---

## Project layout

```
cmd/main.go              CLI entrypoint (flag-based)
cmd/gui/main.go          GUI entrypoint (Gio framework)
cmd/setup/main.go        Lightweight setup loader (raw Win32)
internal/assets/         go:embed DLLs (DWrite.dll, force-proxy.dll)
internal/config/         Default paths, ports, version, repo coordinates
internal/deploy/         Deploy / uninstall / verify DLLs + proxy.txt
internal/discord/        Discord discovery (registry + filesystem)
internal/gui/            Gio UI (dark theme, Russian localization)
internal/installer/      System install, shortcuts (COM), HKCU registry
internal/proxy/          SOCKS5 detection via handshake
internal/update/         GitHub Releases self-update (minio/selfupdate)
third_party/gioui.org/   Vendored, patched Gio
```

---

> **🇷🇺 Документация на русском языке: [README.ru.md](README.ru.md)**
