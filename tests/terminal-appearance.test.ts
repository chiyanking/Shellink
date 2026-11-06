import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createCommandHighlighter, terminalTheme } from '../src/terminal-appearance.ts'

const RESET = '\x1b[0m'
const DARK_COMMAND = '\x1b[1;38;2;239;235;223m'
const DARK_PROMPT = '\x1b[38;2;172;203;152m'
const DARK_PATH = '\x1b[38;2;219;172;131m'

test('dark and light themes use their own canvas instead of a stale CSS variable', () => {
  assert.equal(terminalTheme('dark').background, '#171916')
  assert.equal(terminalTheme('dark').foreground, '#d0cdbf')
  assert.equal(terminalTheme('light').background, '#ffffff')
  assert.equal(terminalTheme('light').foreground, '#414141')
  assert.equal(terminalTheme('dark').green, '#a9c59b')
  assert.equal(terminalTheme('dark').brightWhite, '#efebdf')
})

function luminance(hex: string): number {
  const channels = hex.slice(1).match(/../g)!.map(value => {
    const channel = Number.parseInt(value, 16) / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
}

test('ANSI output stays readable against both terminal canvases', () => {
  const roles = [
    'foreground', 'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
    'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue',
    'brightMagenta', 'brightCyan', 'brightWhite',
  ] as const
  for (const scheme of ['light', 'dark'] as const) {
    const theme = terminalTheme(scheme)
    const background = luminance(theme.background!)
    for (const role of roles) {
      const foreground = luminance(theme[role]!)
      const contrast = (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
      assert.ok(contrast >= 4.5, `${scheme} ${role} contrast is ${contrast.toFixed(2)}`)
    }
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
