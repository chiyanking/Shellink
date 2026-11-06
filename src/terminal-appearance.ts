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
 * Values mirror --terminal-* in aurora.css; both schemes keep a dark terminal.
 */
export const terminalPalettes: Record<ColorScheme, Palette> = {
  light: {
    canvas: '#0d1017',
    foreground: '#b7bfd0',
    muted: '#727d93',
    command: '#eef1f8',
    prompt: '#3ddc97',
    path: '#7c9dff',
    cursor: '#3fd0f0',
    selection: '#333454',
  },
  dark: {
    canvas: '#05060a',
    foreground: '#b7bfd0',
    muted: '#727d93',
    command: '#eef1f8',
    prompt: '#3ddc97',
    path: '#7c9dff',
    cursor: '#3fd0f0',
    selection: '#39405a',
  },
}

type AnsiPalette = Pick<
  ITheme,
  | 'black' | 'red' | 'green' | 'yellow' | 'blue' | 'magenta' | 'cyan' | 'white'
  | 'brightBlack' | 'brightRed' | 'brightGreen' | 'brightYellow'
  | 'brightBlue' | 'brightMagenta' | 'brightCyan' | 'brightWhite'
>

/** Saturated ANSI roles remain readable on the dark terminal in either scheme. */
const ansiPalette: AnsiPalette = {
  black: '#232733', red: '#ff7b8a', green: '#3ddc97', yellow: '#ffc857',
  blue: '#7c9dff', magenta: '#b18cff', cyan: '#3fd0f0', white: '#b7bfd0',
  brightBlack: '#727d93', brightRed: '#ffa2ad', brightGreen: '#82edbd',
  brightYellow: '#ffdc8e', brightBlue: '#a7bcff', brightMagenta: '#cfb6ff',
  brightCyan: '#8ce8fa', brightWhite: '#eef1f8',
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
    selectionInactiveBackground: dark ? '#2c3145' : '#2b3047',
    overviewRulerBorder: p.canvas,
    scrollbarSliderBackground: '#272e3b',
    scrollbarSliderHoverBackground: '#3a4356',
    scrollbarSliderActiveBackground: '#515d75',
    ...ansiPalette,
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
