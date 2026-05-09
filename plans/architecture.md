# discord-szx Architecture

## Overview

CLI-утилита на Go, которая:
1. Обнаруживает установленный Discord ( Stable / PTB / Canary / Development )
2. Копирует `data/` файлы (`DWrite.dll`, `force-proxy.dll`) в каталог Discord
3. Определяет работающие SOCKS5-прокси (v2ray / nekobox) на портах 2080 и 1080
4. Записывает найденный прокси в `proxy.txt` (в каталоге data, а затем копируется в Discord)

---

## Project Structure

```
discord-szx/
├── cmd/
│   └── main.go              # Entrypoint, wiring
├── internal/
│   ├── discord/
│   │   └── finder.go        # Поиск установки Discord
│   ├── deploy/
│   │   └── deployer.go      # Копирование файлов data/ → Discord dir
│   ├── proxy/
│   │   └── detector.go      # Обнаружение SOCKS5 прокси
│   └── config/
│       └── config.go        # Пути, порты, константы
├── data/
│   ├── DWrite.dll           # DLL для инъекции в Discord
│   ├── force-proxy.dll      # DLL принудительного прокси
│   └── proxy.txt            # Шаблон прокси-конфига
├── plans/
│   └── architecture.md      # Этот файл
├── go.mod
└── go.sum
```

---

## Module Details

### 1. `internal/config/config.go` — Конфигурация

```go
package config

type Config struct {
    DataDir      string   // "./data"
    ProxyFile    string   // "proxy.txt"
    ProxyPorts   []int    // [2080, 1080]
    DiscordPaths []string // реестр / известные пути
    DLLFiles     []string // ["DWrite.dll", "force-proxy.dll"]
}
```

- Все настраиваемые параметры в одном месте
- Порты прокси по умолчанию: 2080 (nekobox), 1080 (v2ray)
- Порядок портов важен — первый найденный используется

### 2. `internal/discord/finder.go` — Поиск Discord

**Стратегия поиска (по приоритету):**

1. **Windows Registry** — `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\`
   - Искать по `DisplayName` содержащему `Discord`
   - Извлечь `InstallLocation` или `UninstallString` → получить путь

2. **Известные пути filesystem:**
   ```
   %LocalAppData%\Discord\
   %LocalAppData%\DiscordPTB\
   %LocalAppData%\DiscordCanary\
   %LocalAppData%\DiscordDevelopment\
   ```

3. **Process enumeration** — найти запущенный `Discord.exe`, извлечь каталог

**Возвращаемая структура:**
```go
type DiscordInstall struct {
    Path       string    // e.g. C:\Users\xxx\AppData\Local\Discord
    Version    string    // e.g. app-1.0.9002
    Channel    string    // "stable", "ptb", "canary", "development"
    AppDir     string    // full path to app-x.x.x directory
}
```

**Логика:**
- `FindDiscord() ([]DiscordInstall, error)` — возвращает все найденные установки
- `FindPrimaryDiscord() (*DiscordInstall, error)` — возвращает первую (приоритет: stable > ptb > canary > dev)
- Внутри `AppDir` лежит `Discord.exe` — именно туда нужно копировать DLL

### 3. `internal/deploy/deployer.go` — Деплой файлов

```go
type Deployer struct {
    Config *config.Config
}

func (d *Deployer) Deploy(install *discord.DiscordInstall, proxyAddr string) error
```

**Шаги Deploy():**

1. Прочитать `data/proxy.txt` шаблон
2. Заменить плейсхолдеры на найденный прокси-адрес/порт
3. Записать обновлённый `proxy.txt` в `install.AppDir`
4. Скопировать `data/DWrite.dll` → `install.AppDir/DWrite.dll`
5. Скопировать `data/force-proxy.dll` → `install.AppDir/force-proxy.dll`
6. Проверить что все файлы на месте (verify)

**Обработка ошибок:**
- Discord запущен → предупреждение: файлы могут быть заблокированы, предложить закрыть
- Нет прав записи → предложить запустить от админа
- Файл уже существует → перезаписать (backup старого не-DLL, если размер отличается)

### 4. `internal/proxy/detector.go` — Обнаружение прокси

```go
type ProxyInfo struct {
    Host    string // "127.0.0.1"
    Port    int    // 2080
    Source  string // "nekobox" | "v2ray" | "unknown"
    Alive   bool   // подтверждён коннектом
}

func DetectProxies(ports []int) ([]ProxyInfo, error)
func DetectBestProxy(ports []int) (*ProxyInfo, error)
```

**Стратегия обнаружения:**

1. **TCP connect probe** — попытка `net.DialTimeout("tcp", "127.0.0.1:PORT", 3s)` на каждый порт
   - Порт открыт → прокси вероятно работает

2. **SOCKS5 handshake** — отправить SOCKS5 greeting (`\x05\x01\x00`) на открытый порт
   - Получили `\x05\x00` → это SOCKS5, прокси живой

3. **Process detection (опционально):**
   - `nekobox.exe` → порт 2080
   - `v2ray.exe` / `xray.exe` → порт 1080
   - Через Windows API: `CreateToolhelp32Snapshot` или `tasklist`

**Формат proxy.txt:**
```
SOCKS5_PROXY_ADDRESS=127.0.0.1
SOCKS5_PROXY_PORT=2080
```

### 5. `cmd/main.go` — Главный поток

```
┌─────────────┐
│   Start      │
└──────┬───────┘
       ▼
┌─────────────┐    ┌──────────────┐
│ Config Init  │───▶│ Default paths│
└──────┬───────┘    │ & ports      │
       ▼            └──────────────┘
┌─────────────┐
│Proxy Detect  │──── Try ports 2080, 1080
│              │──── SOCKS5 handshake
└──────┬───────┘
       ▼
┌─────────────┐
│Proxy Found?  │──── No ──▶ Error: no proxy
└──────┬───────┘
      Yes
       ▼
┌─────────────┐
│Discord Find  │──── Registry
│              │──── Filesystem
│              │──── Process
└──────┬───────┘
       ▼
┌─────────────┐
│Discord Found?│──── No ──▶ Error: not installed
└──────┬───────┘
      Yes
       ▼
┌─────────────┐
│   Deploy     │──── Write proxy.txt
│              │──── Copy DWrite.dll
│              │──── Copy force-proxy.dll
└──────┬───────┘
       ▼
┌─────────────┐
│   Verify     │──── All files exist & sized
└──────┬───────┘
       ▼
┌─────────────┐
│    Done ✓    │
└─────────────┘
```

---

## Error Handling Strategy

| Ситуация | Действие |
|----------|----------|
| Нет прокси | Вывести инструкцию: запустить v2ray/nekobox, выйти с кодом 1 |
| Нет Discord | Вывести известные пути, предложить установить, код 2 |
| Discord запущен | Предупредить, предложить `-force` флаг для перезаписи |
| Нет прав | Предложить `Run as Administrator` |
| Файлы заблокированы | Retry с backoff, затем ошибка |

---

## CLI Flags

```
discord-szx [flags]

Flags:
  --proxy-port int    Переопределить порт прокси (0 = авто)
  --proxy-host host   Переопределить хост прокси (default 127.0.0.1)
  --discord-path dir  Переопределить путь к Discord
  --channel string    Выбрать канал: stable|ptb|canary|dev
  --dry-run           Показать что будет сделано без выполнения
  --force             Копировать даже если Discord запущен
  --no-verify         Пропустить проверку после копирования
```

---

## Key Decisions

1. **Go** — нативный бинарник без зависимостей, удобно для Windows
2. **internal/** — приватные пакеты, не экспортируются
3. **SOCKS5 handshake** — надёжнее чем просто TCP connect (отличает прокси от других сервисов на том же порту)
4. **Приоритет портов** — 2080 > 1080 (nekobox первый, т.к. он чаще используется в связке)
5. **Копирование, не перемещение** — `data/` остаётся как источник; в Discord идут копии
6. **proxy.txt генерируется динамически** — шаблон в data/ заменяется на актуальный адрес перед копированием
7. **Реестр Windows** — самый надёжный способ найти Discord на Windows
