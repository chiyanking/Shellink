# Shellink

Shellink is a local desktop terminal workspace built with React, Tauri, and a Rust core. It stores connection profiles, terminal history, MCP configuration, and incoming Webhook messages in one local data directory.

The Rust core is part of the Desktop process. Closing the main window stops every terminal child process and the MCP HTTP listener. There is no separate daemon or sidecar process.

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

## MCP

The Desktop starts MCP HTTP on `127.0.0.1:7070` by default. Set `SHELLINK_HOST`, `SHELLINK_PORT`, or `SHELLINK_HTTP_ENABLED=false` before starting the Desktop to change the listener. The persisted enablement and bearer token are stored in `mcp.json` below the Shellink data directory.

The MCP settings page shows both client configurations. HTTP clients use the URL and bearer token. Stdio clients run the packaged executable with the `mcp` argument; that process forwards JSON-RPC to the running Desktop, so it shares the same sessions and stops working when the Desktop exits.

The current MCP tools cover profiles, session creation/state/output/input, and session close. The Tauri RPC surface also exposes profile editing, session history and replay, Webhook messages, and lifecycle status.

## Data and security

The default data directory is `$HOME/.Shellink` on macOS/Linux and `%APPDATA%\\Shellink` on Windows. Set `SHELLINK_HOME` to use another directory. `master.key`, `mcp.json`, and the SQLite database are created with private permissions where supported.

Command profiles execute their configured command with the Desktop user's permissions. Only use trusted commands and scripts. Keep the MCP listener on loopback unless a network listener is explicitly required, and rotate the MCP token after exposing it to another machine.

## Repository layout

| Path | Purpose |
| --- | --- |
| `src/` | React Desktop interface |
| `src-tauri/src/` | Rust core, Tauri commands, session manager, SQLite store, and MCP HTTP/stdio transport |
| `src-tauri/tauri.conf.json` | Native window and packaging configuration |
| `docs/` | Architecture and Desktop development notes |
