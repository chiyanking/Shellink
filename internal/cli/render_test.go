package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// The renderer is what most people see of the command line, so it gets tested
// the way people meet it: with the answer a few typical commands really return.

// forceColor answers the "what happens in a real terminal" question, which is
// where every invisible escape sequence gets a chance to break alignment.
func forceColor(t *testing.T) {
	t.Helper()
	previous, had := os.LookupEnv("SHELLINK_COLOR")
	_ = os.Setenv("SHELLINK_COLOR", "always")
	ResetColorForTest()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SHELLINK_COLOR", previous)
		} else {
			_ = os.Unsetenv("SHELLINK_COLOR")
		}
		ResetColorForTest()
	})
}

func withoutColor(t *testing.T) *strings.Builder {
	t.Helper()
	previous, had := os.LookupEnv("SHELLINK_COLOR")
	_ = os.Setenv("SHELLINK_COLOR", "never")
	ResetColorForTest()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SHELLINK_COLOR", previous)
		} else {
			_ = os.Unsetenv("SHELLINK_COLOR")
		}
		ResetColorForTest()
	})
	return &strings.Builder{}
}

func TestRenderTableAlignsColumns(t *testing.T) {
	withoutColor(t)
	answer := []any{
		map[string]any{"id": "s1", "state": "WAITING_INPUT", "mode": "AUTO", "profileName": "tencent"},
		map[string]any{"id": "s200", "state": "DISCONNECTED", "mode": "MANUAL", "profileName": "local"},
	}
	lines := strings.Split(render(answer), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected header, rule and two rows, got %d lines:\n%s", len(lines), render(answer))
	}
	header, first, second := lines[0], lines[2], lines[3]
	if !strings.HasPrefix(header, "id ") || !strings.Contains(header, "state") {
		t.Fatalf("header lost its columns: %q", header)
	}
	// Column starts have to match between rows: that alignment is the whole
	// point of using a table instead of JSON.
	stateColumn := strings.Index(header, "state")
	if got := strings.Index(first, "WAITING_INPUT"); got != stateColumn {
		t.Fatalf("state started at %d in the row but %d in the header", got, stateColumn)
	}
	if got := strings.Index(second, "DISCONNECTED"); got != stateColumn {
		t.Fatalf("state started at %d in the second row but %d in the header", got, stateColumn)
	}
}

func TestRenderTableCountsWideCharactersAsTwoColumns(t *testing.T) {
	withoutColor(t)
	answer := []any{
		map[string]any{"name": "abc", "target": "x"},
		map[string]any{"name": "腾讯云服务器", "target": "y"},
	}
	lines := strings.Split(render(answer), "\n")
	first, second := lines[2], lines[3]
	// Compare columns, not byte offsets: a Chinese character is three bytes
	// long in UTF-8 but two columns wide on screen.
	target := visibleWidth(first[:strings.Index(first, "x")])
	if got := visibleWidth(second[:strings.Index(second, "y")]); got != target {
		t.Fatalf("Chinese row started its second column at %d, English row at %d", got, target)
	}
}

func TestRenderObjectAlignsKeys(t *testing.T) {
	withoutColor(t)
	answer := map[string]any{"id": "s1", "running": true, "pid": float64(4242)}
	lines := strings.Split(render(answer), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected one line per field, got %q", render(answer))
	}
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("line %d is empty", index)
		}
	}
	if value := strings.TrimSpace(strings.SplitN(lines[0], "  ", 2)[1]); value != "s1" {
		t.Fatalf("id rendered as %q", value)
	}
	if !strings.Contains(lines[1], "true") {
		t.Fatalf("running lost its value: %q", lines[1])
	}
}

// Color is exactly where a table goes wrong: an escape sequence is invisible on
// screen but counts towards any naive width check, so a colored cell ends up
// truncated or padded by the wrong amount. This is the regression test for it.
func TestRenderTableAlignsColumnsWithColor(t *testing.T) {
	forceColor(t)
	answer := []any{
		map[string]any{"id": "JMQH5KKu", "state": "WAITING_INPUT", "target": "root@server:22"},
		map[string]any{"id": "ab12", "state": "DISCONNECTED", "target": "local"},
	}
	lines := strings.Split(render(answer), "\n")
	if !strings.Contains(lines[2], "JMQH5KKu") {
		t.Fatalf("id was cut short once it carried color: %q", lines[2])
	}
	stateAt := func(line string) int {
		index := strings.Index(line, "WAITING_INPUT")
		if index < 0 {
			index = strings.Index(line, "DISCONNECTED")
		}
		return visibleWidth(ansiAware(line[:index]))
	}
	if first, second := stateAt(lines[2]), stateAt(lines[3]); first != second {
		t.Fatalf("state landed on column %d in one row and %d in the other", first, second)
	}
}

func TestRenderListsNothing(t *testing.T) {
	withoutColor(t)
	if got := strings.TrimSpace(render([]any{})); got != "(none)" {
		t.Fatalf("empty list rendered as %q", got)
	}
	if got := strings.TrimSpace(render(map[string]any{})); got != "(empty)" {
		t.Fatalf("empty object rendered as %q", got)
	}
}

func TestRenderKeepsMultilineOutput(t *testing.T) {
	withoutColor(t)
	answer := map[string]any{"output": "first line\nsecond line", "exitCode": float64(0)}
	text := render(answer)
	if !strings.Contains(text, "first line") || !strings.Contains(text, "second line") {
		t.Fatalf("multi-line output was flattened:\n%s", text)
	}
	if strings.Count(text, "\n") < 2 {
		t.Fatalf("expected one line per output line:\n%s", text)
	}
}

func TestRenderColorsStates(t *testing.T) {
	previous, had := os.LookupEnv("SHELLINK_COLOR")
	_ = os.Setenv("SHELLINK_COLOR", "always")
	ResetColorForTest()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SHELLINK_COLOR", previous)
		} else {
			_ = os.Unsetenv("SHELLINK_COLOR")
		}
		ResetColorForTest()
	})
	text := render(map[string]any{"state": "WAITING_INPUT", "mode": "MANUAL", "active": true})
	if !strings.Contains(text, "\x1b[32m") || !strings.Contains(text, "\x1b[35m") {
		t.Fatalf("states were not colored: %q", text)
	}
	// Color must never leak: every sequence is closed before the next line.
	if !strings.HasSuffix(text, "\x1b[0m") {
		t.Fatalf("escape sequence was left open: %q", text)
	}
}

func TestPrintOutputJSONIsMachineReadable(t *testing.T) {
	withoutColor(t)
	data := captureStdout(t, func() {
		printOutput(map[string]any{"id": "s1", "running": true}, true)
	})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &decoded); err != nil {
		t.Fatalf("--json output is not valid JSON: %v (%q)", err, data)
	}
	if decoded["id"] != "s1" {
		t.Fatalf("json output lost fields: %q", data)
	}
	if strings.Contains(strings.TrimSpace(data), "\n") {
		t.Fatalf("json output must stay on one line: %q", data)
	}
}

func TestPrintOutputHumanFormHasNoBraces(t *testing.T) {
	withoutColor(t)
	data := captureStdout(t, func() {
		printOutput(map[string]any{"id": "s1", "running": true}, false)
	})
	if strings.Contains(data, "{") || strings.Contains(data, "\"") {
		t.Fatalf("human output still looks like JSON: %q", data)
	}
}

func TestColorFlagOverridesEnvironment(t *testing.T) {
	previous, had := os.LookupEnv("SHELLINK_COLOR")
	_ = os.Setenv("SHELLINK_COLOR", "never")
	ResetColorForTest()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SHELLINK_COLOR", previous)
		} else {
			_ = os.Unsetenv("SHELLINK_COLOR")
		}
		ResetColorForTest()
	})
	applyColorPreference(Parse([]string{"--color"}).Flags)
	if !useColor() {
		t.Fatal("bare --color should force color on even when the environment says never")
	}
	applyColorPreference(Parse([]string{"--color", "never"}).Flags)
	if useColor() {
		t.Fatal("--color never should force color off")
	}
}

func TestHelpKeepsEveryCommand(t *testing.T) {
	withoutColor(t)
	for _, topic := range []string{"root", "profile", "session", "server", "webhook", "mcp", "upgrade"} {
		page := help(topic)
		if !strings.Contains(page, "Usage") {
			t.Fatalf("%s help has no usage line:\n%s", topic, page)
		}
	}
	if page := help("root"); !strings.Contains(page, "shellink session list") {
		t.Fatalf("root help lost its examples:\n%s", page)
	}
	if page := help("help session"); !strings.Contains(page, "session") {
		t.Fatalf("unknown topics should fall back to the root page")
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KB", 5 * 1024 * 1024: "5.0 MB"}
	for bytes, want := range cases {
		if got := humanSize(bytes); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", bytes, got, want)
		}
	}
}

// captureStdout runs fn with stdout pointing at a pipe, which is how the
// printer can be checked without touching a global write function.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = writer
	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	os.Stdout = saved
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return string(data)
}
