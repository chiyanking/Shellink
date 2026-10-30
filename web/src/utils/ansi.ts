/**
 * Turn a recorded terminal stream into HTML.
 *
 * The live terminal is xterm.js, which understands escape sequences natively.
 * The history view is a <pre>, which does not — so the colours the remote shell
 * produced have to be converted here or they are lost.
 *
 * Only SGR (colour and attribute) sequences become markup. Cursor moves, screen
 * clears and other control sequences describe a screen that no longer exists,
 * so they are dropped rather than shown as garbage.
 */

const ESC = '\x1b'
const CSI = /^\x1b\[([0-9;:?]*)([A-Za-z])/

/** The 16 colours terminals agree on, as normal + bright rows. */
const PALETTE = [
  '#3b4252', '#bf616a', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#88c0d0', '#d8dee9',
  '#4c566a', '#d08770', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#8fbcbb', '#eceff4',
]

/** The 6x6x6 colour cube, used when a program emits a 256-colour code. */
function cube(index: number): string {
  const levels = [0, 95, 135, 175, 215, 255]
  const b = index % 6
  const g = Math.floor(index / 6) % 6
  const r = Math.floor(index / 36) % 6
  return `rgb(${levels[r]},${levels[g]},${levels[b]})`
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

interface Style {
  bold: boolean
  dim: boolean
  italic: boolean
  underline: boolean
  fg: string
  bg: string
}

function css(style: Style): string {
  const parts: string[] = []
  if (style.bold) parts.push('font-weight:700')
  if (style.dim) parts.push('opacity:.6')
  if (style.italic) parts.push('font-style:italic')
  if (style.underline) parts.push('text-decoration:underline')
  if (style.fg) parts.push(`color:${style.fg}`)
  if (style.bg) parts.push(`background:${style.bg}`)
  return parts.join(';')
}

function blank(): Style {
  return { bold: false, dim: false, italic: false, underline: false, fg: '', bg: '' }
}

/**
 * Apply one SGR parameter list to the running style.
 *
 * `next` walks 38/48 extended colour codes, which carry their own arguments
 * (5;<n> for the 256 palette, 2;<r>;<g>;<b> for true colour).
 */
function apply(params: number[], style: Style): void {
  for (let i = 0; i < params.length; i++) {
    const code = params[i]
    switch (code) {
      case 0:
        Object.assign(style, blank())
        break
      case 1:
        style.bold = true
        break
      case 2:
        style.dim = true
        break
      case 3:
        style.italic = true
        break
      case 4:
        style.underline = true
        break
      case 22:
        style.bold = false
        style.dim = false
        break
      case 23:
        style.italic = false
        break
      case 24:
        style.underline = false
        break
      case 30:
      case 31:
      case 32:
      case 33:
      case 34:
      case 35:
      case 36:
      case 37:
        style.fg = PALETTE[code - 30]
        break
      case 39:
        style.fg = ''
        break
      case 90:
      case 91:
      case 92:
      case 93:
      case 94:
      case 95:
      case 96:
      case 97:
        style.fg = PALETTE[code - 90 + 8]
        break
      case 40:
      case 41:
      case 42:
      case 43:
      case 44:
      case 45:
      case 46:
      case 47:
        style.bg = PALETTE[code - 40]
        break
      case 49:
        style.bg = ''
        break
      case 100:
      case 101:
      case 102:
      case 103:
      case 104:
      case 105:
      case 106:
      case 107:
        style.bg = PALETTE[code - 100 + 8]
        break
      case 38:
        if (params[i + 1] === 5 && params[i + 2] !== undefined) {
          const index = params[i + 2]
          style.fg = index < 16 ? PALETTE[index] : cube(index - 16)
          i += 2
        } else if (params[i + 1] === 2 && params[i + 4] !== undefined) {
          style.fg = `rgb(${params[i + 2]},${params[i + 3]},${params[i + 4]})`
          i += 4
        }
        break
      case 48:
        if (params[i + 1] === 5 && params[i + 2] !== undefined) {
          const index = params[i + 2]
          style.bg = index < 16 ? PALETTE[index] : cube(index - 16)
          i += 2
        } else if (params[i + 1] === 2 && params[i + 4] !== undefined) {
          style.bg = `rgb(${params[i + 2]},${params[i + 3]},${params[i + 4]})`
          i += 4
        }
        break
      default:
        break
    }
  }
}

function parseParams(text: string): number[] {
  if (text === '') return [0]
  return text
    .split(';')
    .map((piece) => (piece === '' ? 0 : Number(piece)))
    .filter((value) => !Number.isNaN(value))
}

export function ansiToHtml(input: string): string {
  if (!input.includes(ESC)) return escapeHtml(input)
  const style = blank()
  let out = ''
  let open = false
  let i = 0

  const close = () => {
    if (open) {
      out += '</span>'
      open = false
    }
  }
  const applyStyle = () => {
    const rules = css(style)
    if (rules === '') return
    out += `<span style="${rules}">`
    open = true
  }

  while (i < input.length) {
    const ch = input[i]
    if (ch === ESC) {
      if (input[i + 1] === '[') {
        const match = CSI.exec(input.slice(i))
        if (match) {
          if (match[2] === 'm') {
            close()
            apply(parseParams(match[1]), style)
            applyStyle()
          }
          // Any other CSI is a screen operation with no meaning in review.
          i += match[0].length
          continue
        }
        i += 2
        continue
      }
      if (input[i + 1] === ']') {
        // OSC (window title and friends) runs until BEL or ST.
        let j = i + 2
        while (j < input.length && input[j] !== '\x07' && !(input[j] === ESC && input[j + 1] === '\\')) {
          j++
        }
        i = input[j] === '\x07' ? j + 1 : j + 2
        continue
      }
      i += 2
      continue
    }
    if (ch === '\r') {
      // CRLF is one newline: drop the CR and let the LF through on the next
      // pass. A bare CR only rewinds a cursor that no longer exists here.
      i++
      continue
    }
    out += escapeHtml(ch)
    i++
  }
  close()
  return out
}
