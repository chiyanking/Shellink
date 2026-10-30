package sessions

import (
	"strings"
	"testing"

	"shellink/internal/store"
)

func out(raw, plain string) store.HistoryChunk {
	return store.HistoryChunk{Direction: "output", DataRaw: raw, DataPlain: plain}
}

func in(raw string) store.HistoryChunk {
	return store.HistoryChunk{Direction: "input", DataRaw: raw, DataPlain: raw}
}

// shown mirrors what the API returns and the history view renders: ANSI escapes
// stripped and CRLF folded to LF.
func shown(s string) string {
	return StripANSI(strings.ReplaceAll(s, "\r\n", "\n"))
}

// An interactive shell echoes every keystroke, so the recorded output stream is
// already the whole transcript. Typed input must not be rendered again (once
// per keystroke) or the history turns into one character per line.
func TestRenderHistoryKeepsEchoedTranscript(t *testing.T) {
	chunks := []store.HistoryChunk{
		out("root@box:~# ", "root@box:~# "),
		in("l"), out("l", "l"),
		in("s"), out("s", "s"),
		in("\r"), out("\r\n", "\n"),
		out("a.txt  b.txt\r\n", "a.txt  b.txt\n"),
		out("\x1b[?2004hroot@box:~# ", "root@box:~# "),
		in("c"), out("c", "c"),
		in("d"), out("d", "d"),
		in(" "), out(" ", " "),
		in("/"), out("/", "/"),
		in("\r"), out("\r\n", "\n"),
	}

	got := shown(renderHistoryWithoutInternal(chunks, "\n"))
	want := "root@box:~# ls" + "\n" + "a.txt  b.txt\n" + "root@box:~# cd /" + "\n"
	if got != want {
		t.Fatalf("transcript mismatch\n got: %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "❯") {
		t.Fatalf("echoed input must not be rendered with the audit marker: %q", got)
	}
}

// Shell exec jobs write the command to the session without a terminal echo, so
// the command has to be rendered explicitly to stay auditable.
func TestRenderHistoryShowsUnEchoedCommand(t *testing.T) {
	chunks := []store.HistoryChunk{
		in("echo hello\n"),
		out("hello\n", "hello\n"),
	}

	got := renderHistoryWithoutInternal(chunks, "\n")
	want := "\n❯ echo hello\nhello\n"
	if got != want {
		t.Fatalf("audit command mismatch\n got: %q\nwant: %q", got, want)
	}
}

// Internal transfer payloads are collapsed into a single marker line.
func TestRenderHistoryCollapsesInternalChunks(t *testing.T) {
	chunks := []store.HistoryChunk{
		out("before\n", "before\n"),
		{Internal: true, Direction: "output", DataRaw: "BASE64...", DataPlain: "BASE64..."},
		{Internal: true, Direction: "output", DataRaw: "BASE64...", DataPlain: "BASE64..."},
		out("after\n", "after\n"),
	}

	got := renderHistoryWithoutInternal(chunks, "\n")
	if !strings.Contains(got, "before\n") || !strings.Contains(got, "after\n") {
		t.Fatalf("surrounding output lost: %q", got)
	}
	if !strings.Contains(got, "hidden 2 internal transfer output chunks") {
		t.Fatalf("internal marker missing: %q", got)
	}
	if strings.Contains(got, "BASE64") {
		t.Fatalf("internal payload leaked: %q", got)
	}
}
