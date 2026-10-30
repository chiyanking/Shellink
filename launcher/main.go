package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type statusResp struct {
	WebUI struct {
		URL string `json:"url"`
	} `json:"webui"`
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("无法定位启动器自身路径:", err)
		pause()
		return
	}
	dir := filepath.Dir(exePath)
	core := filepath.Join(dir, "shellink.exe")
	if _, err := os.Stat(core); err != nil {
		fmt.Println("未找到 shellink.exe，请将其与本启动器放在同一目录下。")
		fmt.Println("目录:", dir)
		pause()
		return
	}

	fmt.Println("正在启动 Shellink 守护进程 ...")
	cmd := exec.Command(core, "server", "start")
	// Run from the folder that holds the binaries: the daemon only ever uses
	// absolute paths, but a stable working directory keeps relative arguments
	// (and any file the user drops next to the executable) predictable.
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	// Warnings the daemon prints (for example "data directory not writable,
	// keeping data beside the executable") must stay visible even when the
	// start succeeds, so stderr is always echoed.
	warning := strings.TrimSpace(stderr.String())
	if err != nil {
		// server start reports the real reason (port in use, unwritable data
		// directory, ...) on stderr. Dropping it left the user with a bare
		// "exit status 1" and no way to tell what went wrong.
		fmt.Println("启动失败:", err)
		if warning != "" {
			fmt.Println()
			fmt.Println(warning)
		}
		if tail := logTail(core, dir); tail != "" {
			fmt.Println()
			fmt.Println("---- 最近日志 ----")
			fmt.Println(tail)
			fmt.Println("------------------")
		}
		if hint := hintFor(stderr.String() + stdout.String()); hint != "" {
			fmt.Println()
			fmt.Println("建议:", hint)
		}
		pause()
		return
	}
	if warning != "" {
		fmt.Println(warning)
	}

	url := "http://127.0.0.1:7070/shellink/ui/"
	var st statusResp
	if err := json.Unmarshal(stdout.Bytes(), &st); err == nil && st.WebUI.URL != "" {
		url = st.WebUI.URL
	}

	fmt.Println("Shellink 已启动 ✓")
	time.Sleep(800 * time.Millisecond)
	openBrowser(url)

	fmt.Println()
	fmt.Println("──────────────────────────────────────────")
	fmt.Println("Web 控制台已打开:", url)
	fmt.Println("守护进程在后台运行中，可安全关闭本窗口。")
	fmt.Println()
	fmt.Println("常用命令（在该目录下打开终端）：")
	fmt.Println("  停止服务 : shellink.exe server stop")
	fmt.Println("  查看状态 : shellink.exe server status")
	fmt.Println("  CLI 终端 : shellink.exe cli")
	fmt.Println("──────────────────────────────────────────")
	fmt.Println("按 Enter 退出本窗口（不影响后台服务）...")
	pause()
}

// logTail returns the last lines of the daemon log so a failed start shows what
// the daemon itself complained about.
func logTail(core, dir string) string {
	cmd := exec.Command(core, "server", "logs", "--lines", "15")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func hintFor(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "address already in use"),
		strings.Contains(lower, "only one usage of each socket address"):
		return "端口已被占用：先运行 shellink.exe server stop，或关掉已在运行的 Shellink 窗口后再启动。"
	case strings.Contains(lower, "access is denied"),
		strings.Contains(lower, "permission denied"):
		return "数据目录不可写：确认杀毒软件没有锁定 %APPDATA%\\Shellink，或用 SHELLINK_HOME 指向一个可写目录。"
	case strings.Contains(lower, "did not start"):
		return "守护进程没有起来：看上方的最近日志，通常是端口冲突或数据目录无权限。"
	}
	return ""
}

func openBrowser(url string) {
	c := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	_ = c.Start()
}

func pause() {
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
}
