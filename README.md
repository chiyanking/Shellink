# shellink

shellink is a local desktop terminal workspace built with React, Tauri, and a Rust core. It stores connection profiles, terminal history, MCP configuration, and incoming Webhook messages in one local data directory.

The Rust core is part of the Desktop process. Closing the main window stops every terminal child process, the MCP HTTP listener, and the local stdio bridge. There is no separate daemon or sidecar process.

## Features

- Local command and OpenSSH connection profiles.
- SSH MFA/2FA with interactive password, verification-code, and challenge-response dialogs.
- Interactive terminal sessions with AUTO and MANUAL input modes.
- Session history, replay, rename, close, and persistent exit status.
- Encrypted profile secrets using a local `master.key`.
- MCP over HTTP at `/shellink/mcp` and MCP stdio through the Desktop executable's `mcp` argument.
- Incoming Webhook capture at `/shellink/webhook/callback`.
- Native Desktop data-directory and lifecycle controls.

Local and SSH sessions run in a native pseudo-terminal (PTY), supporting terminal controls and window resizing. SSH sessions use the platform OpenSSH client. Password and key passphrase prompts are answered once from the encrypted profile fields when the client asks for them. Additional authentication challenges appear in a dedicated dialog, including verification codes, recovery codes, and Duo-style option prompts. Responses pass directly to OpenSSH through a per-session askpass channel and are never saved to profiles or terminal input history. SSH private keys are written to a temporary private file for the lifetime of the session and removed when it ends.

For password-then-code SSH servers, select **密码 + MFA / 2FA**, enter the login password in the profile, and enter the SMS or authenticator code in the second-step dialog when the server requests it. Select **交互认证（MFA / 2FA）** to enter every response when connecting. Existing **密码** and **SSH 密钥** profiles also support additional factors automatically. Follow each server prompt; for push authentication, choose the requested option and approve on your device. Prompts work in both AUTO and MANUAL sessions, expire after five minutes, and can be cancelled by closing the authentication dialog (which closes that connection). This supports factors exposed by the server through SSH keyboard-interactive authentication.

## Requirements

- Node.js 22.19 or newer.
- Rust and the Tauri v2 system dependencies for the target platform.
- OpenSSH client for SSH profiles.

## Development

```sh
npm ci
npm run desktop:dev
```

The regular frontend commands remain available for browser-only work:

```sh
npm run dev
npm run build
npm run test:terminal
```

Rust checks and tests run from the Tauri project:

```sh
cargo check --manifest-path src-tauri/Cargo.toml
cargo test --manifest-path src-tauri/Cargo.toml
```

The MCP integration check runs a built Desktop executable with temporary data directories. It requires a macOS/Linux graphical desktop session and checks both HTTP enablement states, stdio, raw input, recent output snapshots, mode switching, exit tracking, and token rotation:

```sh
cargo build --manifest-path src-tauri/Cargo.toml
python3 scripts/test-mcp.py
```

## MCP

The Desktop starts MCP HTTP on `127.0.0.1:7070` by default. Set `SHELLINK_HOST`, `SHELLINK_PORT`, or `SHELLINK_HTTP_ENABLED=false` before starting the Desktop to change the listener. The persisted enablement and bearer token are stored in the SQLite database (`settings` table). Older versions kept them in `mcp.json` below the data directory; that file is imported automatically on startup and then removed.

The MCP settings page shows both client configurations, including TOML for Codex. HTTP clients use the URL and bearer token. Stdio clients run the packaged executable with the `mcp` argument and forward JSON-RPC through a dedicated loopback bridge. Stdio keeps working when public HTTP is disabled, shares the Desktop's sessions, and requires the Desktop to remain open. Generated stdio configurations include the current data directory and database path. The bridge's address is discovered from the private `mcp-runtime.json` file in the data directory; it contains no bearer token.

The stdio proxy refreshes its token for each request and retries a rejected authentication once, so token rotation requires no proxy restart. Connection and I/O timeouts bound proxy requests. Failed I/O never triggers automatic replay of terminal input.

HTTP uses the stateless JSON response form of Streamable HTTP. Clients send `Content-Type: application/json` and `Accept: application/json, text/event-stream`. Initialization negotiates `2025-06-18` or `2025-03-26`; subsequent HTTP requests include the negotiated `MCP-Protocol-Version`. The stdio bridge also accepts legacy `2024-11-05` clients. Unsupported protocol headers return 400; GET returns 405 because this server does not offer an SSE stream. Requests with an Origin header must match the listener's allowed local/configured origin.

MCP tools return both JSON text content and `structuredContent`, with output schemas and tool annotations. Tool failures set `isError: true` and include `error.code` and `error.message`. Common codes are `INVALID_ARGUMENT`, `SESSION_NOT_FOUND`, `PROFILE_NOT_FOUND`, `MANUAL_MODE`, `AUTH_REQUIRED`, `CANCELLED`, and `OPERATION_FAILED`.

`send` submits `text` to the PTY, automatically adding LF unless it already ends with LF or CR. AI clients can send a command without a trailing newline; empty text sends Enter. `raw` sends `text` verbatim for control keys or other input that must not submit a line; empty text sends nothing. Both tools take only `id` and `text`. Success confirms the input was sent; use `out` to observe the result. Input history stores the actual bytes sent, including any added newline. AUTO/MANUAL are collaboration modes: MANUAL rejects MCP input, and MCP can call `mode` to switch back to AUTO.

`out` reads terminal output with a per-session read position held by the Desktop. Only the session `id` is accepted:

```json
{"id":"session-id"}
```

The read returns the output produced since the previous call, in chronological order. The first read returns the latest 200 lines as context. A single read returns at most 10000 lines; call again to continue draining after a large backlog.

Responses include `text` and `active`; empty `text` means no new output. Reads consume: output already returned is never re-sent, so repeated reads return only new lines. Lines are separated by LF (preserving CRLF), joined across PTY chunks, and kept intact without a character limit. The read position lives in Desktop memory only; restarting the Desktop re-primes it from the latest output on the next read. Concurrent readers share one position per session, so a second reader misses output the first reader consumed — use the `state` tool for a stateless snapshot of the latest output to re-sync. `active` describes whether the session is alive, not whether an individual command has finished. List results use the `profiles` or `sessions` array in their structured object.

The ten MCP tools are `profiles`, `new`, `sessions`, `open`, `state`, `out`, `send`, `raw`, `mode`, and `close`. The Tauri RPC surface also exposes profile editing, session history and replay, Webhook messages, and lifecycle status. A background reaper updates exit status and exit codes independently of UI polling.

## Data and security

The default data directory is `$HOME/.shellink` on macOS/Linux and `%APPDATA%\\shellink` on Windows. Set `SHELLINK_HOME` to use another directory. `master.key` and the SQLite database are created with private permissions where supported.

Command profiles execute their configured command with the Desktop user's permissions. Only use trusted commands and scripts. Keep the MCP listener on loopback unless a network listener is explicitly required, and rotate the MCP token after exposing it to another machine.

## Repository layout

| Path | Purpose |
| --- | --- |
| `src/` | React Desktop interface |
| `src-tauri/src/` | Rust core, Tauri commands, session manager, SQLite store, and MCP HTTP/stdio transport |
| `src-tauri/tauri.conf.json` | Native window and packaging configuration |
| `docs/` | Architecture and Desktop development notes |
