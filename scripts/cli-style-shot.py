"""Render real `shellink` command output into a terminal-style PNG.

It runs the shipped binary, keeps the ANSI colour it really emits, converts the
escape sequences to inline styles and asks a headless Chrome to photograph the
result. Nothing is simulated: every line in the picture came out of the binary.
"""

from __future__ import annotations

import html
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DAEMON_DIR = Path(r"E:\Desktop\Shellink-Windows-x64\Shellink-Windows-x64")
EXE = DAEMON_DIR / "shellink.exe"
OUT = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "docs" / "cli-style.png"

PROMPT_DIR = r"E:\Desktop\Shellink-Windows-x64\Shellink-Windows-x64"
# `--window-size` is given in CSS pixels; `--force-device-scale-factor` only
# decides how many device pixels each of those becomes in the file. Baking the
# scale into the window size as well halves the layout width.
VIEW_W = 1240
SCALE = 2
# A port of our own so the daemon under test never races the one the user is
# already running on :7070.
SPARE_PORT = 7171
MAX_LINES = 46
COMMANDS = [
    ["version"],
    ["server", "status"],
    ["profile", "list"],
    ["session", "list"],
]

# 4-bit palette: index -> (normal, bright)
PALETTE = {
    0: ("#1f2430", "#6b7280"),
    1: ("#f07178", "#ff8b92"),
    2: ("#7fd962", "#a5e844"),
    3: ("#ffb454", "#ffd79c"),
    4: ("#5fa8ff", "#82c8ff"),
    5: ("#c397d8", "#d7b4ea"),
    6: ("#39bae6", "#5fd0f5"),
    7: ("#cbd3de", "#e6ebf2"),
}

FG = {30 + k: v[0] for k, v in PALETTE.items()}
FG.update({90 + k: v[1] for k, v in PALETTE.items()})
BG = {40 + k: v[0] for k, v in PALETTE.items()}
BG.update({100 + k: v[1] for k, v in PALETTE.items()})

ANSI_RE = re.compile(r"\x1b\[([0-9;]*)m")


def wide(rune: str) -> bool:
    """True for characters a terminal draws two columns wide."""
    point = ord(rune)
    return (
        0x1100 <= point <= 0x115F
        or 0x2E80 <= point <= 0xA4CF
        or 0xAC00 <= point <= 0xD7A3
        or 0xF900 <= point <= 0xFAFF
        or 0xFE10 <= point <= 0xFE19
        or 0xFE30 <= point <= 0xFE6F
        or 0xFF00 <= point <= 0xFF60
        or 0xFFE0 <= point <= 0xFFE6
        or 0x1F300 <= point <= 0x1F64F
    )


def escape(text: str) -> str:
    """Escape for HTML while keeping every cell exactly as wide as in a terminal.

    A browser falls back to a proportional CJK font, where a Chinese character
    is not exactly twice as wide as a Latin one, so wide characters are pinned
    to 2ch and spaces are made non-collapsing; without both, aligned columns
    drift apart in the picture even though the binary printed them straight.
    """
    out: list[str] = []
    for rune in text:
        if rune == " ":
            out.append("&nbsp;")
        elif wide(rune):
            out.append(f'<span class="w2">{html.escape(rune, quote=False)}</span>')
        else:
            out.append(html.escape(rune, quote=False))
    return "".join(out)


def to_html(raw: str) -> str:
    """Turn a chunk of terminal output into styled HTML."""
    out: list[str] = []
    classes: list[str] = []
    styles: list[str] = []
    index = 0

    def open_span() -> str:
        parts = " ".join(classes)
        inline = ";".join(styles)
        attr = f' class="{parts}"' if parts else ""
        if inline:
            attr += f' style="{inline}"'
        return f"<span{attr}>"

    for match in ANSI_RE.finditer(raw):
        if match.start() > index:
            out.append(escape(raw[index : match.start()]))
        index = match.end()
        out.append("</span>")
        classes, styles = [], []
        codes = [c for c in match.group(1).split(";") if c != ""]
        if not codes:
            codes = ["0"]
        for code in codes:
            value = int(code)
            if value == 0:
                classes, styles = [], []
            elif value == 1:
                classes.append("b")
            elif value == 2:
                classes.append("dim")
            elif value == 3:
                classes.append("i")
            elif value == 4:
                classes.append("u")
            elif value in FG:
                styles.append(f"color:{FG[value]}")
            elif value in BG:
                styles.append(f"background:{BG[value]}")
            else:
                styles.append(f"color:{FG[7]}")
        out.append(open_span())

    if index < len(raw):
        out.append(escape(raw[index:]))
    out.append("</span>")
    return "".join(out)


def ensureDaemon() -> None:
    """Bring up a throwaway daemon on a spare port.

    Whatever the user is running stays untouched: a second port and its own
    data directory keep this probe from binding :7070 or writing into the data
    home the real daemon owns — the first version of this script did both, and
    the daemon answered "address already in use" for every command.
    """
    result = subprocess.run(
        [str(EXE), "server", "status"],
        cwd=str(DAEMON_DIR),
        env=env(),
        capture_output=True,
    )
    if result.returncode == 0:
        return
    subprocess.run(
        [str(EXE), "server", "start"],
        cwd=str(DAEMON_DIR),
        env=env(),
        capture_output=True,
        check=True,
    )
    for _ in range(40):
        time.sleep(0.25)
        probe = subprocess.run(
            [str(EXE), "server", "status"],
            cwd=str(DAEMON_DIR),
            env=env(),
            capture_output=True,
        )
        if probe.returncode == 0:
            return
    raise SystemExit("daemon did not come up on the spare port")


def env() -> dict[str, str]:
    prepared = dict(os.environ)
    prepared["SHELLINK_COLOR"] = "always"
    prepared["COLUMNS"] = "118"
    prepared["SHELLINK_PORT"] = str(SPARE_PORT)
    prepared["SHELLINK_HOME"] = str(DAEMON_DIR / "shellink-data")
    prepared.pop("NO_COLOR", None)
    return prepared


def run(args: list[str]) -> str:
    prepared = env()
    result = subprocess.run(
        [str(EXE)] + args,
        cwd=str(DAEMON_DIR),
        env=prepared,
        capture_output=True,
    )
    text = result.stdout.decode("utf-8", "replace")
    if result.returncode != 0 and result.stderr:
        text += result.stderr.decode("utf-8", "replace")
    return text.rstrip("\r\n")


def shorten(text: str) -> str:
    """Keep a screenshot readable when a transcript runs long."""
    lines = text.splitlines()
    if len(lines) <= MAX_LINES:
        return text
    kept = lines[:MAX_LINES]
    return "\n".join(kept + [f"… {len(lines) - MAX_LINES} more lines · raise --limit to see more"])


def firstSession() -> str:
    """The id of any session on this daemon, so the transcript has content."""
    probe = subprocess.run(
        [str(EXE), "session", "list", "--json"],
        cwd=str(DAEMON_DIR),
        env=env(),
        capture_output=True,
    )
    try:
        rows = json.loads(probe.stdout.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return ""
    if isinstance(rows, list) and rows and isinstance(rows[0], dict):
        return str(rows[0].get("id") or "")
    return ""


def planned() -> list[list[str]]:
    """The commands to photograph, with history last.

    History is the whole point of the colouring work, so it goes at the bottom
    where the eye ends up — and it is truncated, because a real transcript is
    far longer than any screenshot should be.
    """
    commands = [list(item) for item in COMMANDS]
    sid = firstSession()
    if sid:
        commands.append(["session", "history", sid, "--limit", "400"])
    return commands


def render() -> str:
    ensureDaemon()
    blocks: list[str] = []
    for args in planned():
        label = " ".join(args)
        blocks.append(
            '<div class="cmd">'
            f'<span class="path">{html.escape(PROMPT_DIR)}</span>'
            '<span class="gt">&gt;</span> '
            f'<span class="bin">shellink</span> '
            f'<span class="args">{html.escape(label)}</span>'
            "</div>"
        )
        blocks.append(f'<pre class="out">{to_html(shorten(run(args)))}</pre>')
    return "\n".join(blocks)


PAGE = """<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<style>
  :root {{
    --bg: #0d1117;
    --chrome: #161b22;
    --border: #2a313c;
    --text: #cbd3de;
    --dim: #6b7280;
  }}
  * {{ box-sizing: border-box; }}
  body {{
    margin: 0; padding: 34px; background: #0a0d12;
    font-family: "Cascadia Mono", "Cascadia Code", Consolas, "Microsoft YaHei", monospace;
  }}
  .window {{
    background: var(--bg); border: 1px solid var(--border);
    border-radius: 10px; overflow: hidden;
    box-shadow: 0 18px 50px rgba(0,0,0,.55);
  }}
  .titlebar {{
    height: 34px; background: var(--chrome);
    border-bottom: 1px solid var(--border);
    display: flex; align-items: center; padding: 0 14px;
    color: var(--dim); font-size: 12.5px; letter-spacing: .3px;
  }}
  .dots {{ display: flex; gap: 7px; margin-right: 13px; }}
  .dot {{ width: 11px; height: 11px; border-radius: 50%; }}
  .dot.r {{ background: #ff5f57; }} .dot.y {{ background: #febc2e; }} .dot.g {{ background: #28c840; }}
  .body {{ padding: 16px 20px 20px; color: var(--text); font-size: 13.5px; line-height: 1.62; }}
  .cmd {{ margin-top: 14px; }}
  .cmd:first-child {{ margin-top: 0; }}
  .path {{ color: #7fd962; }}
  .gt {{ color: #ffb454; }}
  .bin {{ color: #5fa8ff; font-weight: 700; }}
  .args {{ color: #e6ebf2; }}
  pre.out {{
    margin: 3px 0 0; font: inherit; white-space: pre; overflow: visible;
  }}
  .w2 {{ display: inline-block; width: 2ch; }}
  .b {{ font-weight: 700; }}
  .dim {{ opacity: .62; }}
  .i {{ font-style: italic; }}
  .u {{ text-decoration: underline; }}
</style>
</head>
<body>
  <div class="window">
    <div class="titlebar">
      <div class="dots"><div class="dot r"></div><div class="dot y"></div><div class="dot g"></div></div>
      Windows PowerShell &nbsp;·&nbsp; Shellink 0.2.4
    </div>
    <div class="body">
{content}
    </div>
  </div>
  <script>
    // The window is sized from the real content height, so the picture never
    // ends in a slab of empty terminal.
    window.addEventListener('load', function () {{
      var box = document.querySelector('.window').getBoundingClientRect();
      document.title = 'H' + Math.ceil(box.height + 68);
    }});
  </script>
</body>
</html>
"""


def browser() -> Path:
    for candidate in (
        Path(r"C:\Program Files\Google\Chrome\Application\chrome.exe"),
        Path(r"C:\Program Files (x86)\Google\Chrome\Application\chrome.exe"),
        Path(r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"),
        Path(r"C:\Program Files\Microsoft\Edge\Application\msedge.exe"),
    ):
        if candidate.exists():
            return candidate
    raise SystemExit("no headless browser found")


def main() -> None:
    if not EXE.exists():
        raise SystemExit(f"missing binary: {EXE}")
    work = Path(tempfile.mkdtemp(prefix="shellink-shot-"))
    try:
        page = work / "page.html"
        page.write_text(PAGE.format(content=render()), encoding="utf-8")
        # Keep the markup next to the picture: reading the inline styles is
        # the only way to check the colours survived all the way through.
        OUT.with_suffix(".html").write_text(page.read_text(encoding="utf-8"), encoding="utf-8")
        OUT.parent.mkdir(parents=True, exist_ok=True)
        chrome = browser()
        probe = subprocess.run(
            [
                str(chrome),
                "--headless=new",
                "--no-sandbox",
                "--disable-gpu",
                f"--user-data-dir={work / 'probe'}",
                "--force-device-scale-factor=2",
                "--virtual-time-budget=2000",
                "--dump-dom",
                f"--window-size={VIEW_W},2000",
                page.as_uri(),
            ],
            capture_output=True,
        )
        match = re.search(r"<title>H(\d+)</title>", probe.stdout.decode("utf-8", "replace"))
        height = int(match.group(1)) if match else 1120

        shot = subprocess.run(
            [
                str(chrome),
                "--headless=new",
                "--no-sandbox",
                "--disable-gpu",
                "--hide-scrollbars",
                f"--user-data-dir={work / 'profile'}",
                "--force-device-scale-factor=2",
                f"--screenshot={OUT}",
                f"--window-size={VIEW_W},{height}",
                page.as_uri(),
            ],
            capture_output=True,
        )
        if shot.returncode != 0 or not OUT.exists():
            raise SystemExit(
                "chrome failed: "
                + shot.stderr.decode("utf-8", "replace")[-1500:]
            )
        print(f"written: {OUT} ({OUT.stat().st_size} bytes) height={height}")
    finally:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    main()
