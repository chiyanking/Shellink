<div align="center">

<img src="web/public/shellink-logo.png" alt="Shellink logo" width="112" />

# Shellink

**Persistent SSH and terminal sessions for people and AI agents.**

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-informational)](#installation)

[Installation](#installation) · [Quick start](#quick-start) · [Agent workflow](#agent-workflow) · [Architecture](#architecture)

</div>

Shellink runs SSH or command-based terminal sessions in a local daemon. The CLI, browser console, and automation tools share those sessions, so work can continue after a client exits. Shellink provides session state, history, file transfer, remote editing, and controls for handing a terminal between automation and a person.

> **Security:** A command profile runs its configured command with the daemon user's permissions. Only use trusted commands and scripts. The HTTP API listens on localhost by default. Before allowing network access, set a strong `SHELLINK_TOKEN` and put the service behind TLS.

## What it does

- Connects to SSH hosts or starts a command in a local pseudo-terminal (PTY). Command profiles can run trusted `expect` scripts for jump hosts and interactive login flows.
- Keeps terminal sessions alive in the daemon and exposes their state, input/output history, and interactive input.
- Runs commands synchronously or as pollable jobs, uploads and downloads files through the session, and applies exact remote file replacements.
- Supports `AUTO` and `MANUAL` session modes so a person can take control for approvals, passwords, or one-time codes.
- Offers a terminal UI, a browser console, REST and WebSocket APIs, plus stable `--json` output for automation.
- Stores profiles and encrypted credentials in SQLite. Standalone builds are available for macOS, Linux, and Windows.

## Installation

### Download a release

Download the asset for your platform from [GitHub Releases](https://github.com/jie123108/Shellink/releases/latest).

On macOS or Linux, the installer downloads the matching release and verifies its checksum:

```sh
curl -fsSL https://raw.githubusercontent.com/jie123108/Shellink/main/install.sh -o /tmp/shellink-install.sh
sh /tmp/shellink-install.sh
```

The installer supports macOS and Linux on x64 and arm64. To install a specific release, add `--version vX.Y.Z`.

On Windows, download `shellink-windows-x64.exe` or `shellink-windows-arm64.exe`, then add its folder to `PATH`. SSH profiles use the system OpenSSH Client. Windows PTY sessions use ConPTY (Windows 10 version 1809 or newer).

Check the installation:

```sh
shellink version
shellink server status --json
```

### Build from source

Requirements:

- Go 1.26 or newer.
- Node.js 22.19 or newer and npm when rebuilding the Web UI.
- OpenSSH Client for SSH sessions.

Build the runtime and embedded Web UI:

```sh
git clone https://github.com/jie123108/Shellink.git
cd Shellink
npm install
npm run build
```

The executable is written to `dist/shellink` or `dist/shellink.exe`. If the embedded assets are already present, the Go binary can also be built directly:

```sh
go build -o dist/shellink ./cmd/shellink
```

Cross-compile a specific target or all supported targets:

```sh
npm run build:binary:windows-x64
npm run build:binary:all
```

## Quick start

Start the user-level daemon. Resource commands also start it automatically when needed.

```sh
shellink server start
```

Create a local shell profile. Use the command for your platform:

```sh
# macOS or Linux
shellink profile create --input - --json <<'JSON'
{"name":"local-shell","connectType":"command","command":"/bin/sh"}
JSON
```

```powershell
# Windows PowerShell
@'
{"name":"local-shell","connectType":"command","command":"powershell.exe"}
'@ | shellink profile create --input - --json
```

Use the profile ID returned by the create command:

```sh
shellink session create --profile PROFILE_ID --json
shellink session exec SESSION_ID --command "echo Shellink is ready" --json
shellink session history SESSION_ID --since 0 --json
shellink session close SESSION_ID --json
```

Replace `PROFILE_ID` and `SESSION_ID` with the IDs from the preceding command output. To open the browser console, visit <http://127.0.0.1:7070/shellink/ui/>.

For an SSH profile, set `connectType` to `ssh` and provide `host`, `username`, and optionally `port` (default `22`). Credentials can be supplied in the profile JSON through `--input FILE` or stdin; never pass passwords, private keys, or passphrases as command-line arguments. Profile credentials are encrypted in the local database.

## CLI

Run `shellink --help` for command groups and `shellink agent-doc` for the full command syntax. The main command groups are:

| Command | Purpose |
| --- | --- |
| `shellink cli` | Open the interactive terminal UI |
| `shellink server start|status|stop|restart|logs|run` | Manage the local daemon |
| `shellink profile list|get|create|update|delete` | Manage connection profiles |
| `shellink session ...` | Create and control terminal sessions |
| `shellink webhook ...` | Manage event callbacks |
| `shellink agent-doc` | Print the full agent command reference |
| `shellink upgrade` | Check for or install a release update |

Common session commands:

```sh
shellink session list --json
shellink session state SESSION_ID --json
shellink session exec SESSION_ID --command "uname -a" --json
shellink session input SESSION_ID --text "yes" --json
shellink session upload SESSION_ID --input ./local.txt --path /remote/file.txt --json
shellink session download SESSION_ID --path /remote/file.txt --output ./download.txt --json
shellink session edit SESSION_ID --input edits.json --json
shellink session close SESSION_ID --json
```

Use `--detach` for long commands or transfers, then poll `session exec-status` and pass the returned job ID. Remote edits use exact text replacements; each match must occur once, edits cannot overlap, and the session must be in `AUTO` mode and `WAITING_INPUT` state.

Session states are `CONNECTING`, `OUTPUTTING`, `WAITING_INPUT`, `IDLE`, and `DISCONNECTED`. Modes are `AUTO` and `MANUAL`.

## Agent workflow

The CLI is the supported local automation interface. Read its built-in reference before integrating:

```sh
shellink agent-doc
```

A typical workflow is to find a profile, create a session, check its state, run a command, inspect output or history, and close the session when finished. Use `--json` for machine-readable results. Wait for `WAITING_INPUT` before sending commands; when a session is in `MANUAL` mode, leave terminal input to the person supervising it.

Shellink includes two optional agent skills in `skills/`:

- `shellink-cli` documents profile, session, input, transfer, and editing workflows.
- `shellink-iterm2-import` scans and imports iTerm2 profiles on macOS.

Install a skill with the [skills CLI](https://skills.sh/), for example:

```sh
npx skills add jie123108/Shellink --skill shellink-cli --agent codex
```

## Web UI and API

The daemon embeds the Vue Web UI and serves it at `/shellink/ui/`. HTTP, WebSocket, and health routes use the `/shellink` prefix:

| Route | Purpose |
| --- | --- |
| `/shellink/ui/` | Browser console |
| `/shellink/api/*` | Profiles, sessions, webhooks, and authentication |
| `/shellink/ws/sessions/{id}` | Live terminal stream |
| `/shellink/ws/events` | Session state events |
| `/shellink/healthz` | Liveness check |
| `/shellink/agent.md` | Agent command reference |

The default HTTP address is `127.0.0.1:7070`. For remote access, bind intentionally, set a strong token, and terminate TLS at a trusted reverse proxy.

## Configuration

Shellink reads configuration from environment variables. User data defaults to `~/.Shellink` on macOS/Linux and `%APPDATA%\Shellink` on Windows.

| Variable | Purpose | Default |
| --- | --- | --- |
| `SHELLINK_HOME` | Data directory | Platform-specific path above |
| `SHELLINK_DB` | SQLite database path | `$SHELLINK_HOME/shellink.db` |
| `SHELLINK_LOG` | Daemon log path | `$SHELLINK_HOME/shellink.log` |
| `SHELLINK_HTTP_ENABLED` | Enable HTTP and WebSocket services | `true` |
| `SHELLINK_HOST` / `SHELLINK_PORT` | HTTP bind address and port | `127.0.0.1:7070` |
| `SHELLINK_TOKEN` | Token for remote API access | `change-me` |
| `SHELLINK_MASTER_KEY` | 32-byte key encoded as 64 hex characters | Generated on first start |
| `SHELLINK_EXEC_TIMEOUT_MS` | Default command timeout | `20000` |
| `SHELLINK_TRANSFER_MAX_BYTES` | Maximum transfer size | `6291456` |

Keep the master key stable for as long as the database contains encrypted credentials. The generated key is stored in the data directory.

## Architecture

`cmd/shellink/main.go` is the executable entry point; `internal/cli` implements CLI behavior, and the other `internal` packages contain the daemon and service implementation.

| Path | Responsibility |
| --- | --- |
| `cmd/shellink/` | Go executable entry point |
| `internal/` | Go CLI, daemon, SSH/PTY sessions, storage, protocol, and HTTP API |
| `internal/httpapi/assets/` | Web assets embedded in the Go binary |
| `web/` | Vue Web UI source; used to rebuild embedded assets |
| `plugin-dsh/` | Optional TypeScript adapter for DeepSeek Harness |
| `skills/` | Agent workflow and iTerm2 import skills |

macOS and Linux clients use a Unix domain socket to reach the daemon. Windows uses an authenticated loopback TCP endpoint. The HTTP API is a separate browser and integration interface.

## Development

```sh
npm run dev                 # Go daemon and Vite Web UI
npm run dev:server          # Go daemon only
npm run dev:web             # Vite Web UI only
npm run build               # Rebuild UI assets and Go executable
go test ./...
go vet ./...
npm run test:plugin-dsh     # Optional DeepSeek Harness adapter
```

## License

Shellink is licensed under the [MIT License](LICENSE).
