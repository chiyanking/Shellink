package sessions

import (
	"strings"
	"testing"

	"shellink/internal/store"
)

func TestColorizePromptsTintsBarePrompt(t *testing.T) {
	got := colorizePrompts("root@VM-0-7-ubuntu:/# ls\r\n")
	if !strings.Contains(got, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("identity not coloured: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;34m/\x1b[0m") {
		t.Fatalf("path not coloured: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;33m#\x1b[0m") {
		t.Fatalf("root sigil not coloured: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;33m#\x1b[0m ls") {
		t.Fatalf("command after the prompt was altered: %q", got)
	}
}

func TestColorizePromptsTintsBracketPrompt(t *testing.T) {
	got := colorizePrompts("[root@VM-0-7-ubuntu ~]# echo hi\r\n")
	if !strings.Contains(got, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("identity not coloured: %q", got)
	}
	if !strings.Contains(got, "\x1b[1;34m~\x1b[0m") {
		t.Fatalf("path not coloured: %q", got)
	}
}

func TestColorizePromptsMarksRootDifferentlyFromUser(t *testing.T) {
	root := colorizePrompts("root@host:/#")
	user := colorizePrompts("user@host:~$")
	if !strings.Contains(root, "\x1b[1;33m#\x1b[0m") {
		t.Fatalf("root sigil not tinted: %q", root)
	}
	if !strings.Contains(user, "\x1b[2m$\x1b[0m") {
		t.Fatalf("user sigil not dimmed: %q", user)
	}
}

func TestColorizePromptsLeavesColouredTextAlone(t *testing.T) {
	// A shell with colour in PS1 already painted this line; re-tinting it
	// would fight with the shell's own choice.
	source := "\x1b[01;32mroot@host\x1b[00m:\x1b[01;34m/\x1b[00m# ls"
	if got := colorizePrompts(source); got != source {
		t.Fatalf("coloured prompt was rewritten:\n got: %q\nwant: %q", got, source)
	}
}

func TestColorizePromptsLeavesOrdinaryTextAlone(t *testing.T) {
	for _, source := range []string{
		"total 24",
		"git@github.com:user/repo.git",
		"http://user@host:8080/path",
		"someone@example.com",
		"-rw-r--r-- 1 root root 1234 Oct 30 12:00 file.txt",
	} {
		if got := colorizePrompts(source); got != source {
			t.Errorf("text was rewritten: %q -> %q", source, got)
		}
	}
}

func TestColorizePromptsHandlesEmptyInput(t *testing.T) {
	if got := colorizePrompts(""); got != "" {
		t.Fatalf("empty input produced %q", got)
	}
}

func TestColorizePromptsTintsEveryLine(t *testing.T) {
	got := colorizePrompts("root@host:/# ls\r\nroot@host:/# cd /var\r\n")
	if strings.Count(got, "\x1b[1;32mroot@host\x1b[0m") != 2 {
		t.Fatalf("expected two tinted prompts: %q", got)
	}
}

func TestHasColorIgnoresPlainResets(t *testing.T) {
	if hasColor("\x1b[0mls") {
		t.Fatal("a reset is not a colour")
	}
	if hasColor("\x1b[39m\x1b[49mls") {
		t.Fatal("default-colour codes are not a colour")
	}
	if !hasColor("\x1b[31mls") {
		t.Fatal("red is a colour")
	}
}

// The two branches of Manager.History, tested through the functions they use:
// the coloured one keeps what the shell painted and adds a prompt tint, the
// plain one is what a machine consumer gets to parse.
func TestHistoryColorFlagDecidesWhatTheTranscriptKeeps(t *testing.T) {
	chunks := []store.HistoryChunk{
		out("root@box:/# ls\r\n", "root@box:/# ls\n"),
		out("\x1b[32ma.txt\x1b[0m\r\n", "a.txt\n"),
	}
	raw := renderHistoryWithoutInternal(chunks, "\n")
	colored := colorizePrompts(raw)
	plain := StripANSI(raw)

	if !strings.Contains(colored, "\x1b[32ma.txt\x1b[0m") {
		t.Fatalf("the shell's own colours must survive the coloured branch: %q", colored)
	}
	if !strings.Contains(colored, "\x1b[1;32mroot@box\x1b[0m") {
		t.Fatalf("a bare prompt should be tinted in the coloured branch: %q", colored)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("the plain branch must not leak escape sequences: %q", plain)
	}
	if !strings.Contains(plain, "root@box:/# ls") || !strings.Contains(plain, "a.txt") {
		t.Fatalf("the plain branch lost content: %q", plain)
	}
}

// A real prompt never arrives alone: the shell brackets the paste and retitles
// the window first. Those are dropped before the prompt is recognised.
func TestColorizePromptsSeesThroughShellNoise(t *testing.T) {
	got := colorizePrompts("\x1b[?2004h\x1b]0;root@VM-0-7-ubuntu: ~\x07root@VM-0-7-ubuntu:~# ls\r\n")
	if !strings.Contains(got, "\x1b[1;32mroot@VM-0-7-ubuntu\x1b[0m") {
		t.Fatalf("prompt hidden behind control sequences was not tinted: %q", got)
	}
	if !strings.Contains(got, "#\x1b[0m ls") {
		t.Fatalf("command text was altered: %q", got)
	}
}

// Bracketed paste and window titles are screen operations; leaving them in
// would print escape soup in a review pane.
func TestColorizePromptsDropsScreenNoise(t *testing.T) {
	got := colorizePrompts("\x1b[?2004h\x1b]0;title\x07plain\r\n")
	if got != "plain\n" {
		t.Fatalf("screen noise survived: %q", got)
	}
}

// Program output colours must survive untouched — only a bare prompt gets
// tinted.
func TestColorizePromptsKeepsProgramColours(t *testing.T) {
	// Only the line ending is normalised; the colour the program asked for is
	// passed through untouched.
	got := colorizePrompts("\x1b[32ma.txt\x1b[0m\r\n")
	if got != "\x1b[32ma.txt\x1b[0m\n" {
		t.Fatalf("program colours altered: %q", got)
	}
}
