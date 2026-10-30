package sessions

import (
	"regexp"
	"strings"
)

// A transcript is a raw terminal stream, so a shell prompt arrives surrounded
// by whatever the shell emitted around it — bracketed paste switches, window
// titles, and plain text when PS1 carries no colour at all. That last case is
// the common one for a root shell reached over SSH, and it is exactly the line
// a reader scans for to tell "here is where I was" from "here is what it
// printed".
var (
	// root@VM-0-7-ubuntu:/#  ·  user@host:~$  ·  user@host:/var/log#
	shellPromptPattern = regexp.MustCompile(`(?m)^((?:\x1b\[[0-9;]*m)*)([A-Za-z0-9_.\-]+@[A-Za-z0-9_.\-]+):([^\s$#]*)([#$])`)
	// [root@VM-0-7-ubuntu ~]#  ·  [user@host /var/log]$
	bracketPromptPattern = regexp.MustCompile(`(?m)^((?:\x1b\[[0-9;]*m)*)\[([A-Za-z0-9_.\-]+@[A-Za-z0-9_.\-]+)\s+([^\]]*)\]([#$])`)
	// Anything an escape sequence can do except paint text.
	controlPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|\x1b\([AB012]`)
)

// colorizePrompts tints an uncoloured shell prompt the way a default Ubuntu
// PS1 does: who and where in green, the working directory in blue, and the
// sigil in yellow for root so a privileged shell is not mistaken for a normal
// one.
//
// Sequences that only move a cursor or retitle a window are dropped first:
// they describe a screen that no longer exists, and a prompt hiding behind
// "\x1b[?2004h" would never be recognised as being at the start of a line.
// Whatever the remote side coloured keeps its own colours.
func colorizePrompts(text string) string {
	if text == "" {
		return ""
	}
	text = dropControlSequences(text)
	return tintPrompts(replayEdit(text))
}

// transcriptPlain is the no-colour twin of colorizePrompts: the same recording
// reduced to the characters a reader actually saw, so a machine consumer and a
// review pane always agree on the content.
func transcriptPlain(text string) string {
	return replayEdit(ansiPattern.ReplaceAllString(text, ""))
}

// ColorizeReplay prepares a raw recording for a one-shot terminal replay: the
// screen noise is flattened exactly as the history view does, and prompts the
// remote shell left uncoloured get the standard tint so a replayed session and
// its live continuation look the same.
func ColorizeReplay(text string) string {
	return colorizePrompts(text)
}

// tintPrompts applies the prompt colours to a stream that no longer carries
// non-SGR control sequences.
func tintPrompts(text string) string {
	text = shellPromptPattern.ReplaceAllStringFunc(text, tint)
	return bracketPromptPattern.ReplaceAllStringFunc(text, tint)
}

// dropControlSequences removes every escape sequence that is not an SGR colour,
// and folds CRLF into LF so the transcript reads as plain lines.
func dropControlSequences(text string) string {
	return controlPattern.ReplaceAllStringFunc(text, func(sequence string) string {
		if isSGRSequence(sequence) {
			return sequence
		}
		return ""
	})
}

// isSGRSequence reports whether a sequence only sets colours and attributes —
// those are the ones worth keeping, because they carry real meaning.
func isSGRSequence(sequence string) bool {
	body, ok := strings.CutPrefix(sequence, "\x1b[")
	if !ok || !strings.HasSuffix(body, "m") {
		return false
	}
	params := strings.TrimSuffix(body, "m")
	if params == "" {
		return true
	}
	for _, part := range strings.Split(params, ";") {
		if part == "" {
			continue
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

// tint receives one whole match: a bare "user@host:path#" or the same preceded
// by SGR codes.
func tint(match string) string {
	if hasColor(match) {
		return match // the shell already chose a colour for this prompt
	}
	groups := shellPromptPattern.FindStringSubmatch(match)
	if groups != nil && len(groups) == 5 {
		return groups[1] + tintIdentity(groups[2]) + dim(":") + tintPath(groups[3]) + tintSigil(groups[4])
	}
	groups = bracketPromptPattern.FindStringSubmatch(match)
	if groups != nil && len(groups) == 5 {
		return groups[1] + dim("[") + tintIdentity(groups[2]) + " " + tintPath(groups[3]) + dim("]") + tintSigil(groups[4])
	}
	return match
}

func tintIdentity(value string) string { return "\x1b[1;32m" + value + "\x1b[0m" }

func tintPath(value string) string {
	if value == "" {
		return ""
	}
	return "\x1b[1;34m" + value + "\x1b[0m"
}

// tintSigil gives root its own colour: a "#" prompt means the next command
// runs with full privileges, which is worth seeing before it is read.
func tintSigil(value string) string {
	if value == "#" {
		return "\x1b[1;33m" + value + "\x1b[0m"
	}
	return dim(value)
}

func dim(value string) string { return "\x1b[2m" + value + "\x1b[0m" }

// hasColor reports whether a run of text carries an actual colour, ignoring
// plain resets: a line that begins with "\x1b[0m" is colourless, and
// bleaching it would erase whatever the shell had already painted on it.
func hasColor(match string) bool {
	for _, sequence := range controlPattern.FindAllString(match, -1) {
		if !isSGRSequence(sequence) {
			continue
		}
		switch sequence {
		case "\x1b[0m", "\x1b[m", "\x1b[39m", "\x1b[49m", "\x1b[22m", "\x1b[23m", "\x1b[24m", "\x1b[27m", "\x1b[28m":
			continue
		default:
			return true
		}
	}
	return false
}

// replayEdit folds a recorded stream into the characters a reader would have
// seen on screen. A carriage return rewinds to the start of the line so later
// text replaces what came before it (progress bars, overtyped input), a
// backspace or delete erases the character before it (the pty echoes every
// editing keystroke), and the remaining control bytes — bell, Ctrl-C, flow
// control — carry no text and vanish. Escape sequences must already be reduced
// to SGR colour (dropControlSequences), which passes through untouched: the
// coloured and the plain transcript both run through here, so they stay the
// same text with and without paint.
func replayEdit(text string) string {
	if !strings.ContainsAny(text, "\x1b\r\b\x7f\x00\x01\x02\x03\x04\x05\x06\x07\x0b\x0c\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1c\x1d\x1e\x1f") {
		return text
	}
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == '\x1b':
			if end := matchSGR(runes[i:]); end > 0 {
				out = append(out, runes[i:i+end]...)
				i += end
			} else {
				i++ // a malformed or non-SGR escape has nothing to show
			}
		case r == '\r' && i+1 < len(runes) && runes[i+1] == '\n':
			out = append(out, '\n')
			i += 2
		case r == '\n' || r == '\t':
			out = append(out, r)
			i++
		case r == '\r':
			out = rewindToLineStart(out)
			i++
		case r == '\b' || r == 0x7f:
			out = eraseBefore(out)
			i++
		case r < 0x20:
			i++ // bell, Ctrl-C and friends: no visible character
		default:
			out = append(out, r)
			i++
		}
	}
	return string(out)
}

// matchSGR reports how many runes at the front of the slice form a complete
// SGR colour sequence; zero when they do not.
func matchSGR(runes []rune) int {
	if len(runes) < 3 || runes[1] != '[' {
		return 0
	}
	j := 2
	for j < len(runes) && (runes[j] == ';' || (runes[j] >= '0' && runes[j] <= '9')) {
		j++
	}
	if j < len(runes) && runes[j] == 'm' {
		return j + 1
	}
	return 0
}

// rewindToLineStart drops the current line, the way a terminal reuses it when
// a carriage return arrives without a newline.
func rewindToLineStart(out []rune) []rune {
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] == '\n' {
			return out[:i+1]
		}
	}
	return out[:0]
}

// eraseBefore removes the last printed character. A run of SGR colour codes
// at the end of the line belongs to the surrounding text, so it is lifted out
// first and put back after the deletion. A backspace never crosses a line
// boundary: erasing at column zero is a no-op on a real terminal, and letting
// it eat the newline would weld two transcript lines together.
func eraseBefore(out []rune) []rune {
	end := len(out)
	for {
		start, ok := trailingSGR(out[:end])
		if !ok {
			break
		}
		end = start
	}
	if end == 0 || out[end-1] == '\n' {
		return out
	}
	kept := out[:end-1]
	return append(kept, out[end:]...)
}

// trailingSGR reports whether the slice ends with a complete SGR sequence and
// where that sequence starts.
func trailingSGR(out []rune) (int, bool) {
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != '\x1b' {
			continue
		}
		if end := matchSGR(out[i:]); end > 0 && i+end == len(out) {
			return i, true
		}
		return 0, false
	}
	return 0, false
}
