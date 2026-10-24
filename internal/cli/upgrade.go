package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/rpc"
)

const releaseRepo = "jie123108/Shellink"

type releaseMetadata struct {
	TagName string `json:"tag_name"`
}

func handleUpgrade(cfg config.Config, flags map[string]string) error {
	current := rpc.Version
	tag := strings.TrimSpace(flags["version"])
	if tag == "" {
		var release releaseMetadata
		if err := fetchJSON("https://api.github.com/repos/"+releaseRepo+"/releases/latest", &release); err != nil {
			return fmt.Errorf("resolve latest release: %w", err)
		}
		tag = release.TagName
	} else if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if tag == "v" {
		return errors.New("release version tag must not be empty")
	}
	target := strings.TrimPrefix(tag, "v")
	asset, err := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if compareVersion(current, target) == 0 {
		printOutput(map[string]any{"current": current, "target": target, "upToDate": true, "updated": false, "checkOnly": flagBool(flags, "check"), "asset": asset}, flagBool(flags, "json"))
		return nil
	}
	if flagBool(flags, "check") {
		printOutput(map[string]any{"current": current, "target": target, "upToDate": false, "updated": false, "checkOnly": true, "asset": asset}, flagBool(flags, "json"))
		return nil
	}
	baseURL := "https://github.com/" + releaseRepo + "/releases/download/" + tag + "/"
	checksums, err := fetchBytes(baseURL+"SHA256SUMS.txt", 1<<20)
	if err != nil {
		return fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := releaseChecksum(string(checksums), asset)
	if err != nil {
		return err
	}
	binary, err := fetchBytes(baseURL+asset, 128<<20)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	actual := sha256.Sum256(binary)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return errors.New("release binary checksum does not match SHA256SUMS.txt")
	}
	if !flagBool(flags, "yes") {
		if !terminalInput(os.Stdin) {
			return errors.New("upgrade requires confirmation; rerun with --yes")
		}
		fmt.Printf("Upgrade Shellink %s to %s? [y/N] ", current, target)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if answer != "y\n" && answer != "Y\n" && answer != "yes\n" && answer != "YES\n" {
			return errors.New("upgrade canceled")
		}
	}
	installPath, err := executablePath()
	if err != nil {
		return err
	}
	wasRunning, err := stopDaemon(cfg)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(installPath), ".shellink-upgrade-*")
	if err != nil {
		return fmt.Errorf("create upgrade file: %w", err)
	}
	tempPath := temp.Name()
	if _, err = temp.Write(binary); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("write upgrade file: %w", err)
	}
	if err := replaceExecutable(tempPath, installPath, cfg, wasRunning); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	printOutput(map[string]any{"current": current, "target": target, "upToDate": false, "updated": true,
		"checkOnly": false, "asset": asset, "path": installPath, "restarted": wasRunning}, flagBool(flags, "json"))
	return nil
}

func releaseAssetName(goos, goarch string) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	switch goos {
	case "darwin", "linux":
		return "shellink-" + goos + "-" + arch, nil
	case "windows":
		return "shellink-windows-" + arch + ".exe", nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}
}

func fetchJSON(url string, out any) error {
	data, err := fetchBytes(url, 2<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func fetchBytes(url string, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "shellink/"+rpc.Version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d %s", response.StatusCode, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeded %d bytes", limit)
	}
	return data, nil
}

func releaseChecksum(sums, asset string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && filepath.Base(strings.TrimPrefix(fields[1], "*")) == asset {
			if len(fields[0]) != sha256.Size*2 {
				return "", errors.New("invalid SHA-256 release checksum")
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", errors.New("invalid SHA-256 release checksum")
			}
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s was not found in SHA256SUMS.txt", asset)
}

func compareVersion(left, right string) int {
	parse := func(value string) []int {
		value = strings.TrimPrefix(value, "v")
		if index := strings.IndexAny(value, "+-"); index >= 0 {
			value = value[:index]
		}
		parts := strings.Split(value, ".")
		result := make([]int, 3)
		for i := 0; i < len(parts) && i < len(result); i++ {
			_, _ = fmt.Sscanf(parts[i], "%d", &result[i])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Abs(path)
}

func stopDaemon(cfg config.Config) (bool, error) {
	connection, err := daemon.Connect(cfg, 500*time.Millisecond)
	if err != nil {
		return false, nil
	}
	_, requestErr := connection.Request("system.stop", nil, 2*time.Second)
	connection.Close()
	if requestErr != nil {
		return false, fmt.Errorf("stop daemon before upgrade: %w", requestErr)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if current, err := daemon.Connect(cfg, 300*time.Millisecond); err != nil {
			return true, nil
		} else {
			current.Close()
		}
	}
	return false, errors.New("daemon did not stop before upgrade")
}

func terminalInput(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
