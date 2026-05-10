# discord-szx

Discord SOCKS5 proxy deployer for Windows. Embeds DLLs into Discord's app directory to force proxy routing through v2ray/nekobox.

## Build

```sh
make          # builds both CLI and GUI to build/
make clean    # removes build/
```

Requires Go 1.26+, Windows only.

## Project Structure

```
cmd/main.go              — CLI entrypoint (flag-based)
cmd/gui/main.go          — GUI entrypoint (Gio framework)
internal/assets/         — go:embed DLLs (DWrite.dll, force-proxy.dll)
internal/config/         — default paths, ports, constants
internal/deploy/         — deploy/uninstall/verify DLLs + proxy.txt into Discord
internal/discord/        — find Discord via registry + filesystem
internal/gui/            — Gio UI (dark theme, Russian localization)
internal/proxy/          — SOCKS5 detection via handshake on ports 2080/1080
```

## Architecture

1. **Proxy detection**: TCP connect + SOCKS5 handshake on 127.0.0.1:2080 (nekobox) then :1080 (v2ray)
2. **Discord finder**: Windows registry (`HKCU\...\Uninstall`) + known `%LocalAppData%` paths, sorted by channel priority (stable > ptb > canary > dev)
3. **Deploy**: Writes `proxy.txt` (SOCKS5_PROXY_ADDRESS/PORT) + copies embedded DLLs into Discord's `app-x.x.x` directory
4. **Uninstall**: Removes proxy.txt + DLLs from Discord directory
5. **GUI**: Gio-based 420x320 window with status cards, install/uninstall buttons, Russian UI

## Key Details

- DLLs are embedded via `go:embed` in `internal/assets/` — no external data/ directory needed
- GUI uses `-H=windowsgui` linker flag to hide console
- Proxy config format: `SOCKS5_PROXY_ADDRESS=127.0.0.1\nSOCKS5_PROXY_PORT=2080`
- Discord running check via `tasklist` before deploy/uninstall
