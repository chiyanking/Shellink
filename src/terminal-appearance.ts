import type { ITheme } from '@xterm/xterm'

export type ColorScheme = 'light' | 'dark'

type Palette = {
  canvas: string
  foreground: string
  muted: string
  command: string
  prompt: string
  path: string
  cursor: string
  selection: string
}

/**
 * Kept in TS rather than read from CSS variables: xterm resolves its theme once
 * at construction time, and child effects run before App's effect adds `.dark`
 * to <html>. Reading the DOM there yields the light palette on a dark startup.
 * Values mirror --terminal-* in desktop.css.
 */
export const terminalPalettes: Record<ColorScheme, Palette> = {
  light: {
    canvas: '#ffffff',
    foreground: '#50596a',
    muted: '#89909d',
    command: '#2c3443',
    prompt: '#267d65',
    path: '#5475c7',
    cursor: '#5c81e2',
    selection: '#cfdcfb',
  },
  dark: {
    canvas: '#121317',
    foreground: '#aeb4c1',
    muted: '#757b8a',
    command: '#d2d6e0',
    prompt: '#8fceb3',
    path: '#93acf4',
    cursor: '#9cb4f5',
    selection: '#39405a',
  },
}

type AnsiPalette = Pick<
  ITheme,
  | 'black' | 'red' | 'green' | 'yellow' | 'blue' | 'magenta' | 'cyan' | 'white'
  | 'brightBlack' | 'brightRed' | 'brightGreen' | 'brightYellow'
  | 'brightBlue' | 'brightMagenta' | 'brightCyan' | 'brightWhite'
>

/** HexHub-like code surface: muted canvas, saturated but readable ANSI roles. */
const ansiPalettes: Record<ColorScheme, AnsiPalette> = {
  light: {
    black: '#2c3443', red: '#c05b60', green: '#267d65', yellow: '#9a6b16',
    blue: '#3b63bd', magenta: '#7d55b5', cyan: '#1f7d92', white: '#d6dae2',
    brightBlack: '#737985', brightRed: '#d9757a', brightGreen: '#3a9c7d',
    brightYellow: '#b5872c', brightBlue: '#5b7edb', brightMagenta: '#9a74d0',
    brightCyan: '#3a9db2', brightWhite: '#f0f2f6',
  },
  dark: {
    black: '#23252c', red: '#e8878b', green: '#8fceb3', yellow: '#d7b473',
    blue: '#93acf4', magenta: '#b09adf', cyan: '#7fc8d8', white: '#c3c8d4',
    brightBlack: '#6b7182', brightRed: '#f2a5a8', brightGreen: '#a9e4c9',
    brightYellow: '#e7c98d', brightBlue: '#adc1fa', brightMagenta: '#c9b6ee',
    brightCyan: '#9edbe7', brightWhite: '#eef0f6',
  },
}

export function terminalTheme(scheme: ColorScheme): ITheme {
  const p = terminalPalettes[scheme]
  const dark = scheme === 'dark'
  return {
    background: p.canvas,
    foreground: p.foreground,
    cursor: p.cursor,
    cursorAccent: p.canvas,
    selectionBackground: p.selection,
    selectionInactiveBackground: dark ? '#2c3145' : '#dde6fa',
    overviewRulerBorder: p.canvas,
    scrollbarSliderBackground: dark ? '#3a3d4a' : '#c9ced9',
    scrollbarSliderHoverBackground: dark ? '#4a4e5e' : '#aeb6c6',
    scrollbarSliderActiveBackground: dark ? '#5a6074' : '#8c96ab',
    ...ansiPalettes[scheme],
  }
}

const RESET = '\x1b[0m'
const CSI = /^\x1b\[[0-?]*[ -/]*[@-~]/
const OSC = /^\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?/
const SGR = /^\x1b\[[0-9;]*m$/
const PROMPT_START = /^([^\s@]+@[^\s:]+:[^\s#$]*[#$])([ \t])/
const PROMPT_PARTS = /^([^\s@]+@[^\s:]+)(:.*?)([#$])$/

function rgb(hex: string): string {
  const value = Number.parseInt(hex.slice(1), 16)
  return `${(value >> 16) & 255};${(value >> 8) & 255};${value & 255}`
}

function paint(color: string, text: string): string {
  return `\x1b[38;2;${rgb(color)}m${text}${RESET}`
}

function paintPrompt(text: string, p: Palette): string {
  const parts = PROMPT_PARTS.exec(text)
  if (!parts) return paint(p.prompt, text)
  return paint(p.prompt, parts[1]) + paint(p.path, parts[2]) + paint(p.prompt, parts[3])
}

/**
 * Adds command-line colour separation to a PTY stream without buffering it.
 * A shell prompt at a line start is repainted (user@host in green, path in
 * blue, sigil in green) and the typed command is switched to a bright, bold
 * foreground; ordinary output keeps the default foreground. The colour is
 * closed at the first line break, so subsequent output is unaffected.
 *
 * Only zero-width SGR sequences are injected, and lines already carrying
 * their own SGR colours (coloured PS1, TUIs) are passed through untouched.
 * State survives chunk boundaries; a prompt split across two chunks simply
 * stays uncoloured rather than corrupting the stream.
 */
export function createCommandHighlighter(scheme: ColorScheme): (raw: string) => string {
  const p = terminalPalettes[scheme]
  const commandOn = `\x1b[1;38;2;${rgb(p.command)}m`
  let atLineStart = true
  let inCommand = false
  let skipLine = false

  return (raw: string): string => {
    let out = ''
    let i = 0
    while (i < raw.length) {
      const ch = raw[i]
      if (ch === '\r' || ch === '\n') {
        if (inCommand) { out += RESET; inCommand = false }
        out += ch
        i += 1
        atLineStart = true
        skipLine = false
        continue
      }
      if (ch === '\x1b') {
        const tail = raw.slice(i)
        const sequence = CSI.exec(tail) ?? OSC.exec(tail)
        const text = sequence ? sequence[0] : ch
        out += text
        i += text.length
        // Escape sequences are zero-width: bracketed-paste prefixes must not
        // stop prompt detection, but a self-coloured prompt must be left alone.
        if (SGR.test(text)) skipLine = true
        continue
      }
      if (atLineStart && !inCommand && !skipLine) {
        const prompt = PROMPT_START.exec(raw.slice(i))
        if (prompt) {
          out += paintPrompt(prompt[1], p) + prompt[2] + commandOn
          i += prompt[0].length
          atLineStart = false
          inCommand = true
          continue
        }
        skipLine = true
      }
      out += ch
      i += 1
      atLineStart = false
    }
    return out
  }
}
