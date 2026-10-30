package sessions

import (
	"regexp"
	"strings"
)

// LiveTinter adds prompt colours to a live terminal stream.
//
// The web terminal must receive the remote output exactly as it arrives —
// every cursor move, screen clear and bracketed-paste switch has to survive,
// because xterm.js replays them — so unlike colorizePrompts nothing is
// stripped here. A prompt is only recognised where it can appear (at the
// start of a line) and only once it is complete; a chunk boundary falling in
// the middle of a prompt is bridged by holding the unterminated tail back
// until the next chunk resolves it.
type LiveTinter struct {
	pending string
}

// NewLiveTinter returns a filter for one terminal connection.
func NewLiveTinter() *LiveTinter { return &LiveTinter{} }

// maxPromptTail bounds how long an unterminated tail may be held while it
// could still grow into a prompt. A real prompt is a few dozen characters;
// anything longer is ordinary output that simply has not reached a newline.
const maxPromptTail = 256

// Filter folds one incoming chunk into the stream and returns everything that
// may be written to the terminal right away.
func (lt *LiveTinter) Filter(chunk string) string {
	buf := lt.pending + chunk
	lt.pending = ""
	var out strings.Builder
	for {
		idx := strings.IndexByte(buf, '\n')
		if idx < 0 {
			break
		}
		out.WriteString(tintLiveLine(buf[:idx+1]))
		buf = buf[idx+1:]
	}
	if buf == "" {
		return out.String()
	}
	// The tail is an unfinished line. A complete prompt can be tinted at once
	// — whatever follows its sigil is command echo, not more prompt.
	if livePromptPattern.MatchString(buf) {
		out.WriteString(tintLiveLine(buf))
		return out.String()
	}
	if len(buf) <= maxPromptTail && livePromptTailPattern.MatchString(buf) {
		lt.pending = buf // could still grow into a prompt; wait for more
		return out.String()
	}
	out.WriteString(buf)
	return out.String()
}

// Pending reports how many bytes are being held back for a possible prompt.
func (lt *LiveTinter) Pending() int { return len(lt.pending) }

// livePromptPattern finds a "user@host:path#" prompt at the start of a line,
// tolerating any control sequences (bracketed paste, window titles, SGR) the
// shell emitted before it.
var livePromptPattern = regexp.MustCompile(
	`^(?:\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|\x1b\([AB012])*` +
		`((?:\x1b\[[0-9;]*m)*)` +
		`([A-Za-z0-9_.\-]+@[A-Za-z0-9_.\-]+):([^\s$#]*)([#$])`)

// livePromptTailPattern matches an unterminated line that could still grow
// into a prompt: it must already contain the user@host separator and consist
// only of characters a prompt is made of. Anything else — a space, a bare
// word — can never become one, so it is released immediately.
var livePromptTailPattern = regexp.MustCompile(
	`^(?:\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\))*` +
		`[A-Za-z0-9_.\-]*@[A-Za-z0-9_.\-]*:?[^\s]*$`)

// tintLiveLine wraps an uncoloured user@host:path# prompt in the same colours
// the history view uses, keeping every escape sequence around it untouched so
// xterm.js still sees the stream the remote side sent.
func tintLiveLine(line string) string {
	loc := livePromptPattern.FindStringSubmatchIndex(line)
	if loc == nil {
		return line
	}
	match := line[loc[0]:loc[1]]
	if hasColor(match) {
		return line // the shell already chose a colour for this prompt
	}
	head := line[loc[0]:loc[2]]     // control sequences before the prompt
	identity := line[loc[4]:loc[5]] // group 2: user@host
	path := line[loc[6]:loc[7]]     // group 3: working directory
	sigil := line[loc[8]:loc[9]]    // group 4: $ or #
	return head + tintIdentity(identity) + dim(":") + tintPath(path) + tintSigil(sigil) + line[loc[9]:]
}
