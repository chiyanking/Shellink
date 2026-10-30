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
	// Fold newlines exactly the way StripANSI does, so the coloured and the
	// plain transcript are the same text with and without paint.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
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
