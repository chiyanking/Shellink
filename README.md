# Shellink

Shellink is a local desktop terminal workspace built with React, Tauri, and a Rust core. It stores connection profiles, terminal history, MCP configuration, and incoming Webhook messages in one local data directory.

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

The MCP integration check runs a built Desktop executable with temporary data directories. It requires a macOS/Linux graphical desktop session and checks both HTTP enablement states, stdio, raw input, output cursors, mode switching, exit tracking, and token rotation:

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

`shellink_send_input` appends a newline by default. Pass `appendNewline: false` to send raw keys such as `"\u0003"` (Ctrl-C), `"\t"`, or `"\u001b[A"` (Up). Sending empty text with `appendNewline: true` sends Enter. AUTO/MANUAL are collaboration modes: MANUAL rejects MCP input, and MCP can call `shellink_set_mode` to switch back to AUTO.

`shellink_read_output` returns chronological plain-text logs, with these options:

| Argument | Behavior |
| --- | --- |
| `afterSeq` | Omit for recent output; use `0` to start from the beginning. |
| `afterOffset` | Resume within a chunk, measured in Unicode characters; defaults to `0`. |
| `limit` | Maximum output chunks per page, default `400`, maximum `10000`. |
| `maxChars` | Maximum returned text characters, default `16000`, maximum `65536`. |
| `timeoutMs` | With `afterSeq`, wait up to `30000` ms for new output or session exit. |

Continue with `afterSeq: nextSeq` and `afterOffset: nextOffset` from the preceding response. Both values are necessary when a large chunk spans pages. Responses include `text`, `hasMore`, `hasEarlier`, `active`, and `timedOut`. `hasEarlier` indicates that a recent-output request omitted older history; `hasMore` indicates unread output after this page. List results use the `profiles` or `sessions` array in their structured object.

The nine MCP tools cover profiles, session creation/state/output/input, input mode, and session close. The Tauri RPC surface also exposes profile editing, session history and replay, Webhook messages, and lifecycle status. A background reaper updates exit status and exit codes independently of UI polling.

## Data and security

The default data directory is `$HOME/.Shellink` on macOS/Linux and `%APPDATA%\\Shellink` on Windows. Set `SHELLINK_HOME` to use another directory. `master.key` and the SQLite database are created with private permissions where supported.

Command profiles execute their configured command with the Desktop user's permissions. Only use trusted commands and scripts. Keep the MCP listener on loopback unless a network listener is explicitly required, and rotate the MCP token after exposing it to another machine.

## Repository layout

| Path | Purpose |
| --- | --- |
| `src/` | React Desktop interface |
| `src-tauri/src/` | Rust core, Tauri commands, session manager, SQLite store, and MCP HTTP/stdio transport |
| `src-tauri/tauri.conf.json` | Native window and packaging configuration |
| `docs/` | Architecture and Desktop development notes |
