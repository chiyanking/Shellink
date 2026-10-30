<div align="center">

<img src="web/public/shellink-logo.png" alt="Shellink logo" width="112" />

# Shellink

**Persistent SSH and terminal sessions for people and AI agents.**

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-informational)](#installation)

[Installation](#installation) · [Quick start](#quick-start) · [Agent workflow](#agent-workflow) · [MCP](#mcp-integration) · [Architecture](#architecture)

</div>

Shellink runs SSH or command-based terminal sessions in a local daemon. The CLI, browser console, and automation tools share those sessions, so work can continue after a client exits. Shellink provides session state, history auditing, file transfer, remote editing, a built-in MCP server, and controls for handing a terminal between automation and a person.

> **Security:** A command profile runs its configured command with the daemon user's permissions. Only use trusted commands and scripts. The HTTP API listens on localhost by default. Before allowing network access, set a strong `SHELLINK_TOKEN` and put the service behind TLS.

## What it does

- Connects to SSH hosts or starts a local command. macOS and Linux use a pseudo-terminal (PTY); Windows uses anonymous pipes by default, with optional ConPTY support. Command profiles can run trusted `expect` scripts for jump hosts and interactive login flows.
- Keeps terminal sessions alive in the daemon and exposes their state, input/output history, and interactive input.
- Runs commands synchronously or as pollable jobs, uploads and downloads files through the session, and applies exact remote file replacements.
- Supports `AUTO` and `MANUAL` session modes so a person can take control for approvals, passwords, or one-time codes.
- Offers a terminal UI, a browser console, REST and WebSocket APIs, MCP over stdio or HTTP, plus stable `--json` output for automation.
- Retains closed-session records and terminal transcripts for history auditing. The dashboard lists recently ended sessions with their end times, newest first.
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

On Windows, download `shellink-windows-x64.exe` or `shellink-windows-arm64.exe`, rename it to `shellink.exe`, and add its folder to `PATH`. SSH profiles use the system OpenSSH Client. Sessions use anonymous pipes by default. To opt into ConPTY, set `SHELLINK_CONPTY=1` in the daemon's environment before starting it; ConPTY requires Windows 10 version 1809 or newer. Pipe-based sessions do not support local terminal resizing.

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

The executable is written to `dist/shellink` or `dist/shellink.exe`. Generated Web UI files stay in `web/dist/`, which is ignored by Git. Run `npm run build` after a fresh checkout or Web UI changes to generate and embed them. The build script enables the `embedui` build tag automatically. Once those assets are present locally, the Go binary can also be built directly:

```sh
go build -tags=embedui -o dist/shellink ./cmd/shellink
```

Without the `embedui` tag, Go development and tests work without frontend build output; the server loads `web/dist/` from disk when available.

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
shellink session state SESSION_ID --json
```

Replace `PROFILE_ID` and `SESSION_ID` with the IDs from the preceding command output. Session creation is asynchronous: repeat `session state` until the state is `WAITING_INPUT`, then run:

```sh
shellink session exec SESSION_ID --command "echo Shellink is ready" --json
shellink session history SESSION_ID --since 0 --json
```

To open the browser console, visit <http://127.0.0.1:7070/shellink/ui/>. Keep the session open to reuse it; when you want to end it, run `shellink session close SESSION_ID --json`. Closing preserves its record and history.

For an SSH profile, set `connectType` to `ssh` and provide `host`, `username`, and optionally `port` (default `22`). Credentials can be supplied in the profile JSON through `--input FILE` or stdin; never pass passwords, private keys, or passphrases as command-line arguments. Profile credentials are encrypted in the local database.

## CLI

Run `shellink --help` for command groups and `shellink agent-doc` for the agent workflow and command reference. The main command groups are:

| Command | Purpose |
| --- | --- |
| `shellink cli` | Open the interactive terminal UI |
| `shellink server ...` | Manage the daemon with `start`, `status`, `stop`, `restart`, `logs`, or `run` |
| `shellink profile ...` | Manage connection profiles with `list`, `get`, `create`, `update`, or `delete` |
| `shellink session ...` | Create and control terminal sessions |
| `shellink webhook ...` | Manage event callbacks |
| `shellink mcp` | Start an MCP server over stdio for an MCP client |
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

Use `--detach` for long commands, transfers, or edits, then poll `session exec-status` with the returned job ID:

```sh
shellink session exec SESSION_ID --command "long-build" --detach --json
shellink session exec-status JOB_ID --since 0 --wait 20000 --json
```

`exec-status` and `exec-cancel` take a job ID (positional or `--job`), not a session ID.

Continue polling with the returned cursor as `--since` until `done` is true. To cancel a job, run `shellink session exec-cancel JOB_ID --json`. Remote edits use exact text replacements; each match must occur once, edits cannot overlap, and the session must be in `AUTO` mode and `WAITING_INPUT` state.

### Transfer strategy

Because sessions run through a PTY (including jump-host sessions), file transfer types bytes into the terminal rather than opening an SFTP channel. Uploads and downloads pick the best available strategy automatically:

| Path | When | Behavior |
| --- | --- | --- |
| Raw stream | Upload, remote has `head` and `sha256sum`/`shasum` | Streams raw bytes into a remote `head -c` staging file after one handshake; throughput is bounded by the link, not by round trips. Verified by size and sha256. |
| Chunked base64 | Upload fallback, or small files | Base64 payload per staging command starts at 3400 bytes and halves adaptively when a line wedges the remote tty (canonical input is capped at 1024 bytes on macOS/BSD, 4096 on Linux). |
| Sliced pull | Download of files over 1 MB | Pulls `tail|head` slices with incremental decoding instead of one giant base64 frame. |
| Single frame | Everything else | One marked base64 frame for small downloads and edits. |

Raw streaming falls back to chunked base64 when verification fails (for example a jump host that mangles 8-bit data), and chunked staging is size-verified before decoding, so a degraded link cannot silently corrupt a file. Very large transfers still need a direct connection or a tunnel; the PTY path is for when only a terminal is available.

Session states are `CONNECTING`, `OUTPUTTING`, `WAITING_INPUT`, `IDLE`, and `DISCONNECTED`. Modes are `AUTO` and `MANUAL`.

### Session lifetime and history

Sessions survive CLI exits and browser disconnects while the daemon is running. Stopping or restarting the daemon ends active sessions; saved records and history remain available.

`session close` ends the terminal process and retains its record. Use `session history SESSION_ID --since 0 --json` or open the session's history audit from the dashboard to review typed commands and program output after it ends. History hides internal transfer payloads by default; add `--include-internal` to inspect them. The history response contains a `cursor`; pass it as `--since` to read the next page.

To permanently delete a session record and its history, run `shellink session remove-record SESSION_ID --json`. If that session is still active, this also closes it. The dashboard also offers record deletion and bulk history cleanup.

## Agent workflow

The CLI is the supported local automation interface. Read its built-in reference before integrating:

```sh
shellink agent-doc
```

A typical workflow is to find a profile, reuse or create a session, check its state, run a command, and inspect output or history. Keep sessions open after completing a task so they can be reused; close them only when the user requests it. Use `--json` for machine-readable results. Wait for `WAITING_INPUT` before sending commands; when a session is in `MANUAL` mode, leave terminal input to the person supervising it.

Shellink includes two optional agent skills in `skills/`:

- `shellink-cli` documents profile, session, input, transfer, and editing workflows.
- `shellink-iterm2-import` scans and imports iTerm2 profiles on macOS.

Install a skill with the [skills CLI](https://skills.sh/), for example:

```sh
npx skills add jie123108/Shellink --skill shellink-cli --agent codex
```

For DeepSeek Harness, the optional [shellink-dsh adapter](plugin-dsh/README.md) provides tools for commands, jobs, transfers, and remote edits with approval controls. See its [Chinese guide](plugin-dsh/README.zh-CN.md) for setup and configuration.

## MCP integration

Shellink includes an MCP (Model Context Protocol) server for listing and creating profiles, listing and creating sessions, checking session state, reading output, sending input, and closing sessions.

### Local client over stdio

Configure an MCP client to launch `shellink mcp`:

```json
{
  "mcpServers": {
    "shellink": {
      "command": "shellink",
      "args": ["mcp"]
    }
  }
}
```

Use the absolute path to the executable if the client cannot find it in `PATH`. The command starts the daemon automatically when needed. Both MCP transports use the daemon's HTTP API, so keep `SHELLINK_HTTP_ENABLED=true`.

### Client over HTTP

Open the Web UI's **MCP** page at <http://127.0.0.1:7070/shellink/ui/mcp> to copy the client configuration, view available tools, enable or disable the HTTP endpoint, or regenerate its token. The endpoint is enabled by default and requires its own generated bearer token, including for local requests. Its configuration is stored in `$SHELLINK_HOME/mcp.json`; this token is separate from `SHELLINK_TOKEN`.

For clients that accept an HTTP URL and authorization headers:

```json
{
  "mcpServers": {
    "shellink": {
      "url": "http://127.0.0.1:7070/shellink/mcp",
      "headers": {
        "Authorization": "Bearer MCP_TOKEN_FROM_WEB_UI"
      }
    }
  }
}
```

The endpoint accepts JSON-RPC 2.0 requests via HTTP POST. Adjust the configuration to your client's format. Use the CLI or DeepSeek Harness adapter for command execution jobs, file transfers, and remote edits.

## Web UI and API

Packaged builds embed the Vue Web UI and serve it at `/shellink/ui/`. The browser console includes profiles, live terminals, session history auditing, webhook messages, and MCP configuration. HTTP, WebSocket, and health routes use the `/shellink` prefix:

| Route | Purpose |
| --- | --- |
| `/shellink/ui/` | Browser console |
| `/shellink/api/*` | Profiles, sessions, history, webhooks, authentication, and MCP configuration |
| `/shellink/mcp` | MCP over HTTP POST; requires the separate MCP token |
| `/shellink/ws/sessions/{id}` | Live terminal stream |
| `/shellink/ws/events` | Session state events |
| `/shellink/healthz` | Liveness check |
| `/shellink/agent.md`, `/shellink/llms.txt` | Agent command reference |

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
| `SHELLINK_REQUIRE_TOKEN_FOR_SENSITIVE_OPS` | Require an API token for sensitive HTTP operations | Automatic: local requests exempt, remote requests require a token |
| `SHELLINK_MASTER_KEY` | 32-byte key encoded as 64 hex characters | Generated on first start |
| `SHELLINK_CONPTY` | Windows terminal transport; `1` enables ConPTY | Unset (anonymous pipes) |
| `SHELLINK_SSH_READY_TIMEOUT_MS` | SSH connection and login readiness timeout | `30000` |
| `SHELLINK_EXEC_TIMEOUT_MS` | Default command timeout | `20000` |
| `SHELLINK_TRANSFER_MAX_BYTES` | Maximum transfer size | `6291456` |
| `SHELLINK_TRANSFER_TIMEOUT_MS` | Default transfer timeout | `120000` |
| `SHELLINK_EDIT_TIMEOUT_MS` | Default remote edit timeout | `25000` |

Set daemon environment variables before starting it; restart the daemon to apply changes. Keep the master key stable for as long as the database contains encrypted credentials. The generated key is stored in the data directory.

## Architecture

`cmd/shellink/main.go` is the executable entry point; `internal/cli` implements CLI behavior, and the other `internal` packages contain the daemon and service implementation.

| Path | Responsibility |
| --- | --- |
| `cmd/shellink/` | Go executable entry point |
| `internal/` | Go CLI, daemon, SSH/PTY sessions, storage, protocol, and HTTP API |
| `internal/mcpserver/` | Shared MCP tools and JSON-RPC implementation for stdio and HTTP |
| `web/` | Vue Web UI source and Go resource package for embedding the build output |
| `web/dist/` | Generated Web assets; ignored by Git and embedded directly when packaging |
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
go test -tags=embedui ./... # Check embedded Web UI after npm run build
go vet ./...
npm run test:plugin-dsh     # Optional DeepSeek Harness adapter
```

During Web UI development, open <http://localhost:5173/shellink/ui/>. Vite proxies API and WebSocket requests to the daemon on port `7070`.

## License

Shellink is licensed under the [MIT License](LICENSE).
