import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createCommandHighlighter, terminalTheme } from '../src/terminal-appearance.ts'

const RESET = '\x1b[0m'
const DARK_COMMAND = '\x1b[1;38;2;238;241;248m'
const DARK_PROMPT = '\x1b[38;2;61;220;151m'
const DARK_PATH = '\x1b[38;2;124;157;255m'

test('dark and light themes use their own canvas instead of a stale CSS variable', () => {
  assert.equal(terminalTheme('dark').background, '#05060a')
  assert.equal(terminalTheme('dark').foreground, '#b7bfd0')
  assert.equal(terminalTheme('light').background, '#0d1017')
  assert.equal(terminalTheme('light').foreground, '#b7bfd0')
  assert.equal(terminalTheme('dark').green, '#3ddc97')
  assert.equal(terminalTheme('dark').brightWhite, '#eef1f8')
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
