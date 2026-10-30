package sessions

import (
	"regexp"
	"strings"
	"testing"
)

func TestLiveTinterTintsPromptInOneChunk(t *testing.T) {
	lt := NewLiveTinter()
	got := lt.Filter("root@VM-0-7-ubuntu:/# ls\n")
	if !strings.Contains(got, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("identity not coloured: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;34m/\x1b[0m") {
		t.Fatalf("path not coloured: %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[1;33m#\x1b[0m ls\n") {
		t.Fatalf("command after the prompt damaged: %q", got)
	}
	if lt.Pending() != 0 {
		t.Fatalf("nothing should be held back: %q", lt.pending)
	}
}

func TestLiveTinterBridgesPromptAcrossChunks(t *testing.T) {
	lt := NewLiveTinter()
	if got := lt.Filter("root@VM-0-7-ubu"); got != "" {
		t.Fatalf("partial prompt was released early: %q", got)
	}
	if got := lt.Filter("ntu:/# "); !strings.Contains(got, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("prompt was not tinted once complete: %q", got)
	}
	if lt.Pending() != 0 {
		t.Fatalf("prompt tail not released: %q", lt.pending)
	}
}

func TestLiveTinterKeepsColouredPromptAlone(t *testing.T) {
	source := "\x1b]0;title\x07\x1b[?2004h\x1b[01;32mroot@host\x1b[00m:\x1b[01;34m/\x1b[00m# "
	lt := NewLiveTinter()
	if got := lt.Filter(source); got != source {
		t.Fatalf("coloured prompt was rewritten:\n got: %q\nwant: %q", got, source)
	}
}

func TestLiveTinterReleasesOrdinaryTail(t *testing.T) {
	lt := NewLiveTinter()
	// No "@", so this can never grow into a prompt: it must go out at once.
	if got := lt.Filter("total 24"); got != "total 24" {
		t.Fatalf("ordinary output was held back: %q", got)
	}
	// A user@host fragment is held only until it is resolved.
	if got := lt.Filter("user@ho"); got != "" {
		t.Fatalf("prompt-shaped fragment was not held: %q", got)
	}
	if got := lt.Filter("st world\n"); got != "user@host world\n" {
		t.Fatalf("resolved non-prompt was damaged: %q", got)
	}
}

func TestLiveTinterKeepsControlSequencesForTheTerminal(t *testing.T) {
	lt := NewLiveTinter()
	got := lt.Filter("\x1b[?2004hroot@vm:/# top\x1b[2J\x1b[H\n")
	if !strings.Contains(got, "\x1b[?2004h") {
		t.Fatalf("bracketed-paste switch was dropped: %q", got)
	}
	if !strings.Contains(got, "\x1b[2J\x1b[H") {
		t.Fatalf("screen-clear sequences were dropped: %q", got)
	}
}

func TestLiveTinterFlushesOverlongTail(t *testing.T) {
	lt := NewLiveTinter()
	long := "user@host:" + strings.Repeat("a", maxPromptTail)
	if got := lt.Filter(long); got != long {
		t.Fatalf("overlong tail should be released, got %d bytes back", len(got))
	}
}

func TestReplayEditCarriageReturnOverwrites(t *testing.T) {
	if got := replayEdit("downloading 40%\rdownloading 90%\rdone\n"); got != "done\n" {
		t.Fatalf("overwritten lines were kept: %q", got)
	}
	if got := replayEdit("abc\r\nx"); got != "abc\nx" {
		t.Fatalf("CRLF must be a plain newline: %q", got)
	}
}

func TestReplayEditBackspaceErases(t *testing.T) {
	if got := replayEdit("cd \b\x07\x07"); got != "cd" {
		t.Fatalf("backspaces did not erase the typing noise: %q", got)
	}
	if got := replayEdit("ab\ncd\x7f"); got != "ab\nc" {
		t.Fatalf("backspace crossed the line boundary: %q", got)
	}
	if got := replayEdit("ab\x7f\x7fc"); got != "c" {
		t.Fatalf("delete did not erase: %q", got)
	}
	if got := replayEdit("hi\x07\x03there"); got != "hithere" {
		t.Fatalf("bell and Ctrl-C left residue: %q", got)
	}
}

func TestReplayEditPreservesColourAroundDeletedCharacter(t *testing.T) {
	got := replayEdit("bin\x1b[0m\x08x")
	if got != "bi\x1b[0mx" {
		t.Fatalf("colour span was not kept intact: %q", got)
	}
}

// The coloured and the plain transcript must be the same text, with and
// without paint — including the keystroke noise a pty echoes back.
func TestHistoryColourAndPlainAgreeOnEditedInput(t *testing.T) {
	raw := "root@VM-0-7-ubuntu:/# cd /\r\ncd \b\b\x07\n\x1b[01;36mbin\x1b[0m\x08x\n"
	colored := colorizePrompts(raw)
	plain := transcriptPlain(raw)
	if stripSGR(colored) != plain {
		t.Fatalf("coloured and plain transcripts disagree:\n color: %q\n plain: %q", stripSGR(colored), plain)
	}
	if strings.ContainsAny(plain, "\r\b\x7f\x07\x03") {
		t.Fatalf("control bytes leaked into the transcript: %q", plain)
	}
}

// stripSGR removes SGR colour codes for content comparisons.
func stripSGR(text string) string {
	return sgrStripPattern.ReplaceAllString(text, "")
}

var sgrStripPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)
