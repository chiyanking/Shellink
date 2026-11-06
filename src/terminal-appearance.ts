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
  selectionInactive: string
  scrollbar: string
  scrollbarHover: string
  scrollbarActive: string
}

/**
 * Kept in TS rather than read from CSS variables: xterm resolves its theme once
 * at construction time, and child effects run before App's effect adds `.dark`
 * to <html>. Reading the DOM there yields the light palette on a dark startup.
 * Values mirror --terminal-* in aurora.css, including a pure-white light terminal.
 */
export const terminalPalettes: Record<ColorScheme, Palette> = {
  light: {
    canvas: '#ffffff',
    foreground: '#3b4254',
    muted: '#7b8497',
    command: '#1b2233',
    prompt: '#26855c',
    path: '#4263eb',
    cursor: '#4263eb',
    selection: '#cdd9fa',
    selectionInactive: '#e3eafd',
    scrollbar: '#c0c7d3',
    scrollbarHover: '#a8b0bf',
    scrollbarActive: '#8d95a6',
  },
  dark: {
    canvas: '#12141a',
    foreground: '#ccd2e0',
    muted: '#8f96ac',
    command: '#e9ecf5',
    prompt: '#8fd0ab',
    path: '#7c9aff',
    cursor: '#a3b8ff',
    selection: '#2c3854',
    selectionInactive: '#222c44',
    scrollbar: '#565f78',
    scrollbarHover: '#6e7890',
    scrollbarActive: '#8990a6',
  },
}

type AnsiPalette = Pick<
  ITheme,
  | 'black' | 'red' | 'green' | 'yellow' | 'blue' | 'magenta' | 'cyan' | 'white'
  | 'brightBlack' | 'brightRed' | 'brightGreen' | 'brightYellow'
  | 'brightBlue' | 'brightMagenta' | 'brightCyan' | 'brightWhite'
>

/** Each scheme preserves ANSI colour roles with contrast against its own canvas. */
const ansiPalettes: Record<ColorScheme, AnsiPalette> = {
  light: {
    black: '#2c3443', red: '#c04a41', green: '#26855c', yellow: '#9a7213',
    blue: '#3b5bdb', magenta: '#8f4a86', cyan: '#0f7f8a', white: '#6b7280',
    brightBlack: '#737d92', brightRed: '#a53a33', brightGreen: '#1e6a4b',
    brightYellow: '#82620f', brightBlue: '#3251c4', brightMagenta: '#7c3f74',
    brightCyan: '#0d6c77', brightWhite: '#1b2233',
  },
  dark: {
    black: '#6f7688', red: '#e29389', green: '#8fd0ab', yellow: '#d9bd85',
    blue: '#7c9aff', magenta: '#c9a3e0', cyan: '#6fc7b2', white: '#ccd2e0',
    brightBlack: '#8f96ac', brightRed: '#eda79d', brightGreen: '#a8dcbf',
    brightYellow: '#e6cf9e', brightBlue: '#9ab0ff', brightMagenta: '#d8bcee',
    brightCyan: '#8fd6c3', brightWhite: '#e9ecf5',
  },
}

export function terminalTheme(scheme: ColorScheme): ITheme {
  const p = terminalPalettes[scheme]
  return {
    background: p.canvas,
    foreground: p.foreground,
    cursor: p.cursor,
    cursorAccent: p.canvas,
    selectionBackground: p.selection,
    selectionInactiveBackground: p.selectionInactive,
    overviewRulerBorder: p.canvas,
    scrollbarSliderBackground: p.scrollbar,
    scrollbarSliderHoverBackground: p.scrollbarHover,
    scrollbarSliderActiveBackground: p.scrollbarActive,
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
 * A shell prompt at a line start is repainted using separate prompt and path
 * colours, and the typed command is switched to a bold command
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
