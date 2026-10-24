# shellink-dsh

Optional [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) bundle that exposes a local [Shellink](https://github.com/jie123108/Shellink) daemon as typed agent tools over its HTTP API.

The plugin is a **client**. It does not embed the Shellink session engine, Web UI, or credential store. Disposing the plugin closes its HTTP client; remote sessions stay alive on the daemon.

## Security

The plugin asks through DeepSeek Harness `ctx.approval` before:

- opening a session (`shellink_session_open`, default: always)
- a command that matches the danger classifier (`rm`, `kill`, `chmod`, `systemctl restart`, `curl | sh`, `bash -c`, writes to `/etc`, …) or, on a production profile, any command that is not read-only
- `shellink_session_input` that is not a short answer (`y` / `n` / …) — same classifier, so it cannot bypass exec
- creating/updating a `command` profile (runs on the daemon host) or a production SSH host
- switching a session from `MANUAL` back to `AUTO`
- uploading/editing a sensitive remote path, or downloading a secret path (`.ssh`, `.pem`, `.env`, `/etc/shadow`, …)

`profile_delete` and `session_close` are off by default (`approval.gateProfileDelete` / `approval.gateSessionClose`).

Grants are one-shot (`allowed-once`). The plugin keeps an in-process memo per agent session for the same profile and the exact same command string; it is discarded when the process exits. There is no disk allowlist.

This classifier is a heuristic against accidents, not a security boundary. Isolation still depends on the Shellink profile account and `MANUAL` mode. Prefer non-production profiles.

Download writes bytes through the daemon after the plugin checks the local path with `ctx.fs.resolve`. That is a policy check, not the same fence as a `ctx.fs` write.

## Install

Requires a running or installable `shellink` CLI on `PATH`, and DeepSeek Harness `0.1.1-rc.2` (`next` dist-tag on most `@deepseek-ai/dsh-*` packages).

From a published package:

```sh
dsh plugin --profile web add shellink-dsh
```

From this checkout (after `npm install && npm run build --workspace=plugin-dsh`). The plugin uses the Go daemon's local HTTP API and works on Windows, macOS, and Linux.

If `dsh` is on PATH (published CLI):

```sh
dsh plugin --profile web add .
```

If you run DeepSeek Harness from a source checkout, there is no global `dsh` binary. Run `pnpm dsh` from that repo and pass this package as an absolute path (`.` would mean the harness root). Profile directories are pnpm workspaces, so `add` needs `-w`:

```sh
cd /path/to/deepseek-harness
pnpm dsh plugin --profile web add -w /path/to/Shellink/plugin-dsh
```

Restart the profile after adding a bundle. Confirm the rows landed:

```sh
# published CLI
dsh web --dump-config
# source checkout
pnpm dsh web --dump-config
```

You should see `id: shellink` and `id: shellink-tools`.

## Credentials

Profile create/update tools accept `password_ref`, `private_key_ref`, and `passphrase_ref` — POSIX names, never plaintext. Put the values in `$DSH_HOME/.credentials.yaml` or the process environment first. The plaintext is sent only in the local HTTP request and never returned in the tool result, so it does not enter the harness session log.

## Configuration

`cordis.patch.yml` / profile overlay `config` for `id: shellink`:

| key | default | meaning |
|---|---|---|
| `baseUrl` | `$SHELLINK_HTTP_URL` or `http://127.0.0.1:$SHELLINK_PORT` (`7070`) | Local daemon HTTP API |
| `autoStartDaemon` | `true` | Run `shellink server start` when the HTTP API is unavailable |
| `daemonReadyTimeoutMs` | `10000` | Wait for the daemon after spawn |
| `requestTimeoutMs` | `30000` | Per-request timeout |
| `waitReadyTimeoutMs` | `30000` | Wait for `AUTO` + `WAITING_INPUT` |
| `execDefaultTimeoutMs` | `20000` | Daemon exec timeout |
| `uploadMaxBytes` | `6291456` | Local file read cap |
| `profileWrites` | `true` | Register create / update / delete profile tools |
| `approval.mode` | `enforce` | `enforce` asks through `ctx.approval`; `off` disables the gate |
| `approval.defaultTier` | `standard` | `production` treats every host as production (allowlist mode) |
| `approval.productionPatterns` | `["(?i)prod", "(?i)prd"]` | Regexes matched against profile name / host / target / uniqueId. A leading `(?i)` becomes the JS `i` flag |
| `approval.productionProfileIds` | `[]` | Profile ids always treated as production |
| `approval.connect` | `always` | `always` / `production-only` / `off` |
| `approval.remember` | `connect+command` | In-process memo: `connect+command` / `connect` / `off` |
| `approval.memoMaxEntries` | `200` | LRU cap for remembered grants |
| `approval.extraDangerPatterns` | `[]` | Extra command regexes that always ask (beats `allowPatterns`) |
| `approval.allowPatterns` | `[]` | Command regexes that skip the built-in classifier |
| `approval.readOnlyExtra` | `[]` | Extra program names treated as read-only on production hosts |
| `approval.gateProfileDelete` | `false` | Ask before `shellink_profile_delete` |
| `approval.gateSessionClose` | `false` | Ask before `shellink_session_close` |
| `approval.reasonMaxChars` | `400` | Truncate the approval reason shown to the user |

Set `profileWrites: false` to keep list/get only. Invalid `approval.*Patterns` regexes throw when the tools plugin loads.

## Tools

`shellink_profile_list` / `get` / `create` / `update` / `delete`, `shellink_session_list` / `open` / `state` / `history` / `input` / `exec` / `mode` / `close`, `shellink_session_upload` / `download` / `edit`.

Long `exec` / transfer / edit calls take `run_in_background`. Collect them with the harness `job_output` / `job_kill` tools.
