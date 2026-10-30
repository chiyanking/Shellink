package cli

import (
	"os"
	"strings"
	"sync"
	"unicode"
)

// The command line used to answer with raw JSON. That is perfect for a script
// (and stays untouched: --json prints exactly what it printed before) and poor
// for a human, who has to read egg-shaped braces to learn whether a session is
// still alive.
//
// This file carries the small palette the rest of the CLI paints with. It is
// deliberately plain: no library, no table engine, and nothing is emitted when
// the output is not a terminal, so piping into a file or another program stays
// byte-for-byte what it was.

// Each escape is the SGR code pair used to start and end a run of text.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiPurple = "\x1b[35m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[90m"
	ansiInvert = "\x1b[7m"
)

var colorEnabled struct {
	once  sync.Once
	value bool
}

// useColor reports whether escape sequences should be written.
//
// The answer is never cached across processes, but inside one process it is
// decided once: a TTY check is cheap but not free, and the value must not flip
// halfway through a table. NO_COLOR wins over everything, which is the rule
// every modern tool follows; SHELLINK_COLOR=always|never and CLICOLOR_FORCE
// exist for terminals that lie about themselves.
func useColor() bool {
	colorEnabled.once.Do(func() {
		switch strings.ToLower(os.Getenv("SHELLINK_COLOR")) {
		case "always", "1", "true", "yes":
			colorEnabled.value = true
			return
		case "never", "0", "false", "no", "none":
			colorEnabled.value = false
			return
		}
		if os.Getenv("NO_COLOR") != "" {
			colorEnabled.value = false
			return
		}
		if os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0" {
			colorEnabled.value = true
			return
		}
		if !isTTY(os.Stdout) {
			colorEnabled.value = false
			return
		}
		// TERM is unset on Windows consoles, which understand ANSI once virtual
		// terminal processing is turned on, so only an explicit "dumb" opts out.
		if strings.EqualFold(os.Getenv("TERM"), "dumb") {
			colorEnabled.value = false
			return
		}
		colorEnabled.value = true
		enableConsoleColors()
	})
	return colorEnabled.value
}

// ResetColorForTest drops the cached decision. Tests flip SHELLINK_COLOR
// between cases, so the once-per-process answer has to be recomputed.
func ResetColorForTest() {
	colorEnabled.once = sync.Once{}
}

func paint(code, value string) string {
	if value == "" || !useColor() {
		return value
	}
	return code + value + ansiReset
}

func bold(value string) string       { return paint(ansiBold, value) }
func dim(value string) string        { return paint(ansiDim, value) }
func gray(value string) string       { return paint(ansiGray, value) }
func red(value string) string        { return paint(ansiRed, value) }
func green(value string) string      { return paint(ansiGreen, value) }
func yellow(value string) string     { return paint(ansiYellow, value) }
func blue(value string) string       { return paint(ansiBlue, value) }
func purple(value string) string     { return paint(ansiPurple, value) }
func cyan(value string) string       { return paint(ansiCyan, value) }
func keyText(value string) string    { return paint(ansiBold+ansiCyan, value) }
func headText(value string) string   { return paint(ansiBold+ansiGray, value) }
func strongText(value string) string { return paint(ansiBold, value) }

// Symbols a human scans for before reading a word. They are only decorative:
// anything relying on them also gets words.
const (
	symbolOK      = "\u2713" // ✓
	symbolFail    = "\u2717" // ✗
	symbolWarn    = "\u26a0" // ⚠
	symbolPointer = "\u203a" // ›
)

func okSymbol() string { return green(symbolOK) }

// ErrorLine renders a failure for stderr: red and marked, so a failing command
// never reads like ordinary output pasted into a log.
func ErrorLine(text string) string {
	if text == "" || !useColor() {
		return text
	}
	return failSymbol() + " " + red(text)
}
func failSymbol() string { return red(symbolFail) }
func warnSymbol() string { return yellow(symbolWarn) }

// stateText colors a session state. Values come from internal/sessions, but a
// new one must still print legibly, so anything unknown falls back to plain.
func stateText(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "WAITING_INPUT":
		return green(state)
	case "CONNECTING":
		return yellow(state)
	case "OUTPUTTING":
		return cyan(state)
	case "IDLE":
		return yellow(state)
	case "DISCONNECTED":
		return gray(state)
	case "":
		return gray("UNKNOWN")
	default:
		return state
	}
}

// modeText colors AUTO vs MANUAL: MANUAL means a human holds the terminal, and
// everything automatic is refused, which is worth noticing at a glance.
func modeText(mode string) string {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "AUTO":
		return cyan(mode)
	case "MANUAL":
		return bold(purple(mode))
	default:
		return mode
	}
}

// boolText speaks in color before it speaks in letters.
func boolText(flag bool) string {
	if flag {
		return green("true")
	}
	return gray("false")
}

// statusText colors any field that holds one of the known verbs, so
// "running", "ok" and "upToDate" agree wherever they appear.
func statusText(word string) string {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "running", "active", "ok", "ready", "success", "updated", "healthy", "connected":
		return green(word)
	case "stopped", "stopping", "failed", "error", "closed", "cancelled", "canceled", "offline", "missing":
		return red(word)
	case "pending", "starting", "waiting", "retrying", "paused":
		return yellow(word)
	default:
		return word
	}
}

// isTTY reports whether writes to file land on a character device (a console)
// rather than a pipe or a file. The same question works on Windows consoles,
// which is why it lives here instead of a platform file.
func isTTY(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// visibleWidth is the number of terminal columns a string occupies.
//
// Rune counts are not enough here: Chinese labels are two columns wide in
// every terminal, and a table padded by rune count ends up ragged exactly when
// the data is not English.
func visibleWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeWidth(r)
	}
	return width
}

// eastAsian has to be spelled out: the unicode package exposes only some of
// these blocks, and the ones it hides (full-width forms, CJK compatibility) are
// exactly the ones Chinese output reaches for. Ranges come from the East Asian
// Wide/Fullwidth columns of Unicode.
var eastAsian = [][2]rune{
	{0x1100, 0x115F}, // Hangul Jamo
	{0x2E80, 0x303E}, // CJK radicals and symbols
	{0x3041, 0x33FF}, // Hiragana, Katakana, compatibility
	{0x3400, 0x4DBF}, // CJK extension A
	{0x4E00, 0x9FFF}, // CJK unified ideographs
	{0xA000, 0xA4CF}, // Yi
	{0xAC00, 0xD7A3}, // Hangul syllables
	{0xF900, 0xFAFF}, // CJK compatibility ideographs
	{0xFE30, 0xFE6F}, // CJK compatibility forms
	{0xFF00, 0xFF60}, // full-width forms
	{0xFFE0, 0xFFE6}, // full-width signs
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me):
		// Combining marks sit on top of the preceding cell.
		return 0
	default:
		for _, table := range eastAsian {
			if r >= table[0] && r <= table[1] {
				return 2
			}
		}
		return 1
	}
}

// padRightVisible pads to a *column* width and stops at zero.
func padRightVisible(value string, width int) string {
	for visibleWidth(value) > width && width >= 0 {
		runes := []rune(value)
		value = string(runes[:max(0, len(runes)-1)])
	}
	return value + strings.Repeat(" ", max(0, width-visibleWidth(value)))
}

// fitVisible pads or cuts a string that already carries color.
//
// A colored cell has to be measured by what shows on screen - the escape
// sequences are invisible but very much counted by any width check - and it has
// to be cut without losing the color that is already open, which would tint the
// rest of the line. So the leading sequences are taken aside, the visible part
// is trimmed to the column budget with an ellipsis, and the color is closed.
func fitVisible(value string, width int) string {
	plain := ansiAware(value)
	if visibleWidth(plain) <= width {
		return value + strings.Repeat(" ", width-visibleWidth(plain))
	}
	if width <= 1 {
		return strings.Repeat(".", max(0, width))
	}
	prefix := 0
	for prefix < len(value) && value[prefix] == 0x1b {
		end := prefix
		for end < len(value) && value[end] != 'm' {
			end++
		}
		if end < len(value) {
			end++
		}
		prefix = end
	}
	body := value[prefix:]
	var kept []rune
	used := 0
	for _, r := range body {
		next := runeWidth(r)
		if used+next > width-1 {
			break
		}
		kept = append(kept, r)
		used += next
	}
	out := value[:prefix] + string(kept) + "\u2026"
	if prefix > 0 {
		out += ansiReset
	}
	return out
}

// padRightAnsi pads text that already carries escape sequences. The colors must
// survive, so nothing is truncated here - only ever widened - which is all the
// status bar needs and keeps this from trying to cut a sequence in half.
func padRightAnsi(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-visibleWidth(ansiAware(value))))
}
