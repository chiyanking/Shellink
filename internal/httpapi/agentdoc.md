# Shellink AI Agent CLI

Use the local `shellink` CLI as the stable automation interface. Do not implement its local IPC or MessagePack protocol directly. All commands start the local daemon automatically. Add `--json` for stable machine-readable output.

## Workflow

    shellink profile list --query '<name, host, IP, or command keyword>' --json
    shellink session create --profile '<profile-id>' --json
    shellink session state '<session-id>' --json
    shellink session exec '<session-id>' --command 'uname -a' --json
    shellink session exec '<session-id>' --command 'long-build' --detach --json
    shellink session exec-status '<job-id>' --since <cursor> --wait 20000 --json
    shellink session exec-cancel '<job-id>' --json
    shellink session history '<session-id>' --since 0 --json
    shellink session input '<session-id>' --text 'yes' --json
    shellink session close '<session-id>' --json

States are CONNECTING, OUTPUTTING, WAITING_INPUT, IDLE, and DISCONNECTED. Wait for WAITING_INPUT before exec. For IDLE, inspect output and use input only when the program is clearly waiting for an answer. If a session is MANUAL, a human controls it; wait for AUTO rather than forcing input. Keep sessions open after completing a task so they can be reused. Close a session only when the user explicitly requests it.

## Long-running operations

Host agents typically wait only about 30 seconds for a shell command. Keep each CLI call short and use `--detach` for longer operations. Default timeouts are 20 seconds for exec, 25 seconds for edit, and 120 seconds for transfers. The CLI request timeout is the server timeout plus one second.

    shellink session exec '<id>' --command 'long-build' --detach --json
    shellink session exec-status '<job-id>' --since <cursor> --wait 20000 --json
    shellink session exec-cancel '<job-id>' --json

Poll exec-status until `done` is true. Its response includes job status, incremental output, and a cursor. A timed-out exec may still be running and holds the session lock. On a conflict, poll the named job or cancel it instead of retrying blindly. Detached download accepts `--output` so the daemon writes the result to that local path. If a command returns METHOD_NOT_FOUND, run `shellink server restart`.

## Profiles

Search by name, SSH host/IP, or command before creating a session:

    shellink profile list --query '<keyword>' --json
    shellink profile get '<profile-id>' --json

Create or update profiles with `--input FILE` or `--input -`. Credentials must never be passed in argv. Profile JSON uses `connectType: "ssh"` or `connectType: "command"` and supports terminal dimensions and `promptRegex`. Command profiles run with the daemon user's permissions and can use expect for multi-hop connections. Only use trusted commands.

## Sessions and files

    shellink session upload '<id>' --input ./file --path /remote/file --json
    shellink session download '<id>' --path /remote/file --output ./file --json
    shellink session edit '<id>' --input edits.json --json

For large or slow transfers, append `--detach` and poll the job with exec-status. Edit input contains a path and one or more exact replacements:

    {"path":"/remote/file","edits":[{"oldText":"exact text","newText":"replacement"}]}

Remote edits require an AUTO session in WAITING_INPUT. Each oldText must be non-empty and match exactly once, including whitespace. Multiple edits must not overlap. Pass credentials only through JSON supplied with `--input FILE` or `--input -`, never command-line arguments.

## Upgrade

Upgrade the standalone binary from GitHub Releases. Use `--yes` in non-interactive environments:

    shellink upgrade --check --json
    shellink upgrade --yes --json
    shellink upgrade --version v0.2.0 --yes --json

Exit codes are 0 for success, 1 for runtime/service errors, and 2 for argument or validation errors.
