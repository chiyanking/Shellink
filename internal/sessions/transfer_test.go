package sessions

import (
	"bytes"
	"testing"
)

func TestExtractMarkedPayloadRequiresOwnLines(t *testing.T) {
	got, err := extractMarkedPayload("echo SPB_token\nSPB_token\nYWJj\nSPE_token\n", "SPB_token", "SPE_token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "YWJj" {
		t.Fatalf("payload = %q, want YWJj", got)
	}
	if _, err := extractMarkedPayload("SPB_token_suffix\nYWJj\nSPE_token_extra", "SPB_token", "SPE_token"); err == nil {
		t.Fatal("expected marker frame not found")
	}
}

func TestReplaceTextPreservesBOMAndLineEndings(t *testing.T) {
	original := append([]byte{0xef, 0xbb, 0xbf}, []byte("alpha\r\nbeta\r\n")...)
	updated, count, err := replaceText(original, []TextEdit{
		{OldText: "alpha\nbeta", NewText: "first\nsecond"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !bytes.Equal(updated, append([]byte{0xef, 0xbb, 0xbf}, []byte("first\r\nsecond\r\n")...)) {
		t.Fatalf("replacement = %q (%d), unexpected BOM/newline handling", updated, count)
	}
}

func TestReplaceTextRejectsAmbiguousAndOverlappingEdits(t *testing.T) {
	if _, _, err := replaceText([]byte("same same"), []TextEdit{{OldText: "same", NewText: "new"}}); err == nil {
		t.Fatal("expected duplicate text to fail")
	}
	if _, _, err := replaceText([]byte("abcdef"), []TextEdit{
		{OldText: "abc", NewText: "x"},
		{OldText: "cde", NewText: "y"},
	}); err == nil {
		t.Fatal("expected overlapping edits to fail")
	}
	if _, _, err := replaceText([]byte{0xff}, []TextEdit{{OldText: "x", NewText: "y"}}); err == nil {
		t.Fatal("expected invalid UTF-8 to fail")
	}
}
