package cli

import (
	"strings"
	"testing"
)

func TestReleaseAssetName(t *testing.T) {
	cases := map[string]string{
		"darwin/arm64":  "shellink-darwin-arm64",
		"linux/amd64":   "shellink-linux-x64",
		"windows/amd64": "shellink-windows-x64.exe",
		"windows/arm64": "shellink-windows-arm64.exe",
	}
	for platform, want := range cases {
		parts := strings.Split(platform, "/")
		goos, goarch := parts[0], parts[1]
		got, err := releaseAssetName(goos, goarch)
		if err != nil || got != want {
			t.Fatalf("releaseAssetName(%s, %s) = %q, %v; want %q", goos, goarch, got, err, want)
		}
	}
}

func TestReleaseChecksumAndVersionComparison(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if got, err := releaseChecksum(digest+"  ./shellink-linux-x64\n", "shellink-linux-x64"); err != nil || got != digest {
		t.Fatalf("releaseChecksum = %q, %v", got, err)
	}
	if _, err := releaseChecksum("not-a-checksum  other\n", "shellink-linux-x64"); err == nil {
		t.Fatal("expected missing checksum error")
	}
	if compareVersion("v1.4.0", "1.3.9") != 1 || compareVersion("1.3.9", "v1.4.0") != -1 || compareVersion("1.4.0", "v1.4.0") != 0 {
		t.Fatal("version comparison returned an incorrect ordering")
	}
}
