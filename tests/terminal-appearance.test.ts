import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createCommandHighlighter, terminalTheme } from '../src/terminal-appearance.ts'

const RESET = '\x1b[0m'
const DARK_COMMAND = '\x1b[1;38;2;233;236;245m'
const DARK_PROMPT = '\x1b[38;2;143;208;171m'
const DARK_PATH = '\x1b[38;2;124;154;255m'

test('dark and light themes use their own canvas instead of a stale CSS variable', () => {
  assert.equal(terminalTheme('dark').background, '#12141a')
  assert.equal(terminalTheme('dark').foreground, '#ccd2e0')
  assert.equal(terminalTheme('light').background, '#ffffff')
  assert.equal(terminalTheme('light').foreground, '#3b4254')
  assert.equal(terminalTheme('dark').green, '#8fd0ab')
  assert.equal(terminalTheme('dark').brightWhite, '#e9ecf5')
})

function luminance(hex: string): number {
  const channels = hex.slice(1).match(/../g)!.map(value => {
    const channel = Number.parseInt(value, 16) / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
}

function contrast(fg: string, bg: string): number {
  const a = luminance(fg); const b = luminance(bg)
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
}

test('dark-family ANSI roles stay readable against both canvases', () => {
  const roles = [
    'foreground', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan',
    'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue',
    'brightMagenta', 'brightCyan',
  ] as const
  // white/brightWhite are excluded: they intentionally keep standard ANSI
  // semantics (light) for TUI-painted backgrounds, and are covered by the
  // ANSI-black contrast test below.
  for (const scheme of ['light', 'dark'] as const) {
    const theme = terminalTheme(scheme)
    for (const role of roles) {
      const value = contrast(theme[role]!, theme.background!)
      assert.ok(value >= 4.5, `${scheme} ${role} canvas contrast is ${value.toFixed(2)}`)
    }
  }
})

test('white-family ANSI roles stay readable on a TUI-painted ANSI black background', () => {
  // TUIs (k9s, htop, vim) fill their background with ANSI black and render
  // primary text via the white roles; black itself stays dark so that
  // painted background keeps its contrast.
  for (const scheme of ['light', 'dark'] as const) {
    const theme = terminalTheme(scheme)
    for (const role of ['white', 'brightWhite'] as const) {
      const value = contrast(theme[role]!, theme.black!)
      assert.ok(value >= 4.5, `${scheme} ${role} on ANSI black is ${value.toFixed(2)}`)
    }
    const ansiRoles = [
      'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
      'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue',
      'brightMagenta', 'brightCyan', 'brightWhite',
    ] as const
    const lums = ansiRoles.map(role => luminance(theme[role]!))
    assert.ok(lums.every(l => l >= luminance(theme.black!)), `${scheme} black stays darkest`)
    assert.ok(lums.every(l => l <= luminance(theme.brightWhite!)), `${scheme} brightWhite stays brightest`)
    assert.ok(luminance(theme.brightWhite!) > luminance(theme.white!), `${scheme} brightWhite stays brighter than white`)
    assert.ok(luminance(theme.white!) > luminance(theme.brightBlack!), `${scheme} white stays brighter than brightBlack`)
  }
})

test('replay separates prompt, command, and output by colour', () => {
  const highlight = createCommandHighlighter('dark')
  const out = highlight(
    'Welcome to Ubuntu 24.04 LTS\n' +
    'root@VM-0-7-ubuntu:~# ls\r\n' +
    'pt_install_log  install.sh  test.txt\r\n',
  )
  assert.ok(out.startsWith('Welcome to Ubuntu 24.04 LTS\n'), 'plain output is untouched')
  assert.ok(out.includes(`${DARK_PROMPT}root@VM-0-7-ubuntu${RESET}`), 'user@host uses prompt colour')
  assert.ok(out.includes(`${DARK_PATH}:~${RESET}${DARK_PROMPT}#${RESET}`), 'path and sigil use their own colours')
  assert.ok(out.includes(`${DARK_COMMAND}ls${RESET}\r\n`), 'command is bold/bright and closed before the line break')
  assert.ok(
    out.endsWith('pt_install_log  install.sh  test.txt\r\n'),
    'command output keeps the default foreground',
  )
})

test('command colour survives chunk boundaries and resets at the first line break', () => {
  const highlight = createCommandHighlighter('dark')
  const first = highlight('root@VM-0-7-ubuntu:~# ')
  const typed = highlight('l') + highlight('s')
  const end = highlight('\r\n') + highlight('done\r\n')
  assert.ok(first.endsWith(DARK_COMMAND))
  assert.equal(typed, 'ls')
  assert.equal(end, `${RESET}\r\ndone\r\n`)
})

test('self-coloured prompts and bracketed paste prefixes stay intact', () => {
  const coloured = createCommandHighlighter('dark')(
    '\x1b[01;32mroot@host\x1b[00m:~$ ls\r\n',
  )
  assert.ok(coloured.includes('\x1b[01;32mroot@host\x1b[00m'), 'existing SGR is preserved')
  assert.ok(!coloured.includes(DARK_COMMAND), 'no double colouring')

  const bracketed = createCommandHighlighter('dark')('\x1b[?2004hroot@host:~$ ls\r\n')
  assert.ok(bracketed.startsWith('\x1b[?2004h'), 'private-mode sequence is preserved')
  assert.ok(bracketed.includes(DARK_COMMAND), 'prompt is still detected after a zero-width escape')
})
