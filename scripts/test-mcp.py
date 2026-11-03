#!/usr/bin/env python3
"""Exercise the built Desktop and stdio proxy in isolated temporary data homes.

Requires a macOS/Linux graphical desktop session (Tauri) and a built executable.
Usage: python3 scripts/test-mcp.py [path/to/shellink-desktop]
"""
import http.client
import json
import os
from pathlib import Path
import select
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time


ROOT = Path(__file__).resolve().parents[1]
BINARY = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else ROOT / "src-tauri/target/debug/shellink-desktop"


def wait_until(check, timeout=10):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(0.025)
    raise AssertionError("timed out waiting for Desktop MCP")


def exercise(http_enabled):
    with tempfile.TemporaryDirectory(prefix="shellink-mcp-integration-") as directory:
        home = Path(directory)
        database = home / "shellink.db"
        token = "integration-initial-token"
        with sqlite3.connect(database) as db:
            db.execute("CREATE TABLE settings (key TEXT PRIMARY KEY,value TEXT NOT NULL)")
            db.execute("INSERT INTO settings VALUES ('mcp',?)", (json.dumps({"enabled": http_enabled, "token": token}),))
        with socket.socket() as port_socket:
            port_socket.bind(("127.0.0.1", 0))
            port = port_socket.getsockname()[1]
        env = dict(os.environ, SHELLINK_HOME=str(home), SHELLINK_HOST="127.0.0.1", SHELLINK_PORT=str(port), SHELLINK_HTTP_ENABLED="true")
        for key in ("SHELLINK_DB", "SHELLINK_MASTER_KEY", "SHELLINK_PROCESS_MODE"):
            env.pop(key, None)
        with (home / "desktop.log").open("w+") as desktop_log, (home / "stdio.log").open("w+") as stdio_log:
            desktop = subprocess.Popen([str(BINARY)], env=env, stdout=desktop_log, stderr=subprocess.STDOUT)
            proxy = None
            try:
                wait_until(lambda: (home / "mcp-runtime.json").exists())
                assert desktop.poll() is None
                proxy = subprocess.Popen([str(BINARY), "mcp"], env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stdio_log, text=True, bufsize=1)
                request_id = 0

                def rpc(method, params=None):
                    nonlocal request_id
                    request_id += 1
                    request = {"jsonrpc": "2.0", "id": request_id, "method": method}
                    if params is not None:
                        request["params"] = params
                    proxy.stdin.write(json.dumps(request) + "\n")
                    proxy.stdin.flush()
                    assert select.select([proxy.stdout], [], [], 5)[0], "stdio response timed out"
                    response = json.loads(proxy.stdout.readline())
                    assert response["id"] == request_id, response
                    assert "error" not in response, response
                    return response["result"]

                def tool(tool_name, **arguments):
                    result = rpc("tools/call", {"name": "shellink_" + tool_name, "arguments": arguments})
                    assert json.loads(result["content"][0]["text"]) == result["structuredContent"]
                    return result

                init = rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "shellink-integration", "version": "1"}})
                assert init["protocolVersion"] == "2025-06-18"
                proxy.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
                proxy.stdin.flush()
                tools = rpc("tools/list")["tools"]
                assert len(tools) == 9 and not any(tool["name"] == "shellink_screen" for tool in tools)
                assert all("annotations" in item and "outputSchema" in item for item in tools)
                read_tool = next(item for item in tools if item["name"] == "shellink_read_output")
                assert set(read_tool["inputSchema"]["properties"]) == {"id", "limit"}
                profile = tool("create_profile", name="integration", connectType="command", command="exec /bin/sh")["structuredContent"]
                session = tool("create_session", profileId=profile["id"])["structuredContent"]["id"]
                # The typed command contains octal escapes, so PTY echo cannot fake the marker.
                tool("send_input", id=session, text=r"printf '\115\103\120\137\122\101\127\137\117\113\n'", appendNewline=False)
                time.sleep(0.1)
                before = tool("read_output", id=session)["structuredContent"]
                assert "MCP_RAW_OK" not in before["text"], before
                tool("send_input", id=session, text="", appendNewline=True)
                def marker_output():
                    snapshot = tool("read_output", id=session)["structuredContent"]
                    return snapshot if "MCP_RAW_OK" in snapshot["text"] else None

                latest = wait_until(marker_output)
                assert set(latest) == {"text", "hasEarlier", "active"}, latest
                assert "MCP_RAW_OK" in tool("read_output", id=session)["structuredContent"]["text"]
                tail = tool("read_output", id=session, limit=1)["structuredContent"]
                assert len(tail["text"].splitlines()) <= 1 and tail["hasEarlier"] is True, tail
                assert "MCP_RAW_OK" in tool("read_output", id=session)["structuredContent"]["text"]
                tool("set_mode", id=session, mode="MANUAL")
                denied = tool("send_input", id=session, text="pwd")
                assert denied["structuredContent"]["error"]["code"] == "MANUAL_MODE"
                assert tool("set_mode", id=session, mode="AUTO")["structuredContent"]["mode"] == "AUTO"
                tool("send_input", id=session, text="exit 7")

                # Poll SQLite directly: exit tracking must work without a UI/MCP state query.
                def exited():
                    with sqlite3.connect(database) as db:
                        state, code = db.execute("SELECT state,exit_code FROM sessions WHERE id=?", (session,)).fetchone()
                    return state == "DISCONNECTED" and code == 7
                wait_until(exited)
                state = tool("session_state", id=session)["structuredContent"]
                assert state["active"] is False and state["exitCode"] == 7
                token = "integration-rotated-token"
                with sqlite3.connect(database) as db:
                    db.execute("UPDATE settings SET value=? WHERE key='mcp'", (json.dumps({"enabled": http_enabled, "token": token}),))
                assert rpc("ping") == {}  # Same proxy process, no restart after rotation.

                def request_http(method="POST", extra=None, chunked=False, rpc_request=None):
                    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=3)
                    headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-06-18"}
                    headers.update(extra or {})
                    body = json.dumps(rpc_request or {"jsonrpc": "2.0", "id": 1, "method": "ping"}).encode()
                    connection.request(method, "/shellink/mcp", iter([body[:10], body[10:]]) if chunked else body, headers, encode_chunked=chunked)
                    response = connection.getresponse()
                    status, payload = response.status, response.read()
                    connection.close()
                    return status, payload

                if http_enabled:
                    assert request_http()[0] == 200
                    status, payload = request_http(rpc_request={"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "shellink_read_output", "arguments": {"id": session}}})
                    assert status == 200
                    assert json.loads(payload)["result"]["structuredContent"] == tool("read_output", id=session)["structuredContent"]
                    assert request_http("GET")[0] == 405
                    assert request_http(extra={"Origin": "http://evil.invalid"})[0] == 403
                    assert request_http(extra={"MCP-Protocol-Version": "invalid"})[0] == 400
                    assert request_http(extra={"Authorization": "Bearer integration-initial-token"})[0] == 401
                    assert request_http(chunked=True)[0] == 200
                else:
                    try:
                        request_http()
                    except ConnectionRefusedError:
                        pass
                    else:
                        raise AssertionError("public HTTP must stay disabled")
                proxy.stdin.close()
                assert proxy.wait(timeout=5) == 0
                print(f"PASS: HTTP {'enabled' if http_enabled else 'disabled'}, stdio, raw input, recent output snapshots, mode switching, exit tracking, token rotation")
            except Exception:
                desktop_log.seek(0)
                stdio_log.seek(0)
                print(desktop_log.read(), stdio_log.read(), file=sys.stderr)
                raise
            finally:
                if proxy and proxy.poll() is None:
                    proxy.terminate()
                    proxy.wait(timeout=5)
                desktop.terminate()
                try:
                    desktop.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    desktop.kill()
                    desktop.wait(timeout=5)


if __name__ == "__main__":
    if not BINARY.is_file():
        raise SystemExit(f"Build Desktop first; executable missing: {BINARY}")
    for enabled in (True, False):
        exercise(enabled)
