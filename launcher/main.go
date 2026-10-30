package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultURL = "http://127.0.0.1:7070/shellink/ui/"

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

	// Shellink keeps one daemon per data directory and always binds the same
	// port, so a daemon left over from a previous run guarantees a failed
	// start ("Access is denied" on the log/database handle, or the port being
	// taken). Cleaning up first makes every double-click a fresh start.
	fmt.Println("正在检查是否有旧的 Shellink 服务 ...")
	stopExisting(core, dir)

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
		printSelfCheck(dir)
		if path := writeDiagnostics(core, dir, err, stdout.String(), warning); path != "" {
			fmt.Println()
			fmt.Println("诊断信息已写入:", path)
		}
		pause()
		return
	}
	if warning != "" {
		fmt.Println(warning)
	}

	url := defaultURL
	var st statusResp
	if err := json.Unmarshal(stdout.Bytes(), &st); err == nil && st.WebUI.URL != "" {
		url = st.WebUI.URL
	}

	fmt.Println("Shellink 已启动 ✓")
	time.Sleep(800 * time.Millisecond)
	if os.Getenv("SHELLINK_NO_BROWSER") == "" {
		openBrowser(url)
	}

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

// stopExisting shuts down any Shellink daemon that is still alive before this
// one starts.
//
// A graceful "server stop" is tried first because it lets the running daemon
// flush sessions and release the database and log handles. If it does not exit
// in time (hung shutdown, or a daemon living in another folder with a
// different data directory) the processes are killed outright.
func stopExisting(core, dir string) {
	pids := runningShellink()
	if len(pids) == 0 {
		fmt.Println("  没有旧服务在运行")
		return
	}
	fmt.Printf("  发现旧服务 PID %s，正在通知其退出 ...\n", joinPIDs(pids))
	runQuiet(core, dir, "server", "stop")
	if waitGone(pids, 6*time.Second) {
		fmt.Println("  旧服务已正常停止")
		return
	}
	left := aliveOf(pids)
	fmt.Printf("  旧服务未在 6 秒内退出 (PID %s)，强制结束 ...\n", joinPIDs(left))
	for _, pid := range left {
		forceKill(pid)
	}
	if waitGone(pids, 5*time.Second) {
		fmt.Println("  旧服务已强制结束")
		return
	}
	fmt.Printf("  警告: 仍有进程未退出 (PID %s)，启动可能失败。\n", joinPIDs(aliveOf(pids)))
}

func runQuiet(core, dir string, args ...string) {
	cmd := exec.Command(core, args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()
}

func waitGone(pids []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if len(aliveOf(pids)) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func aliveOf(pids []int) []int {
	var left []int
	for _, pid := range pids {
		if alive(pid) {
			left = append(left, pid)
		}
	}
	return left
}

func joinPIDs(pids []int) string {
	parts := make([]string, len(pids))
	for i, pid := range pids {
		parts[i] = fmt.Sprint(pid)
	}
	return strings.Join(parts, ", ")
}

// printSelfCheck reports the few things that decide whether a start can work.
// It runs only after a failure, so the window that the user is looking at
// already contains the answer instead of a generic suggestion.
func printSelfCheck(dir string) {
	fmt.Println()
	fmt.Println("---- 自检 ----")
	// "Cannot write the data directory" has two very different causes: the
	// folder itself is locked, or this process may not write the user profile
	// at all (low integrity token). Probing %TEMP% tells them apart.
	tempBlocked := false
	if probe, err := os.CreateTemp("", "shellink-write-*"); err != nil {
		tempBlocked = true
		fmt.Println("进程写权限: 连临时目录都写不了 → 程序文件夹被标成了「低完整性(Low)」")
	} else {
		name := probe.Name()
		_ = probe.Close()
		_ = os.Remove(name)
		fmt.Println("进程写权限: 可以写临时目录 ✓")
	}
	home := dataHome()
	fmt.Println("数据目录:", home)
	if home == "" {
		fmt.Println("  无法确定: APPDATA / SHELLINK_HOME 都是空的")
	} else if err := os.MkdirAll(home, 0700); err != nil {
		fmt.Println("  创建失败:", err)
	} else if probe, err := os.CreateTemp(home, ".probe-*"); err != nil {
		fmt.Println("  不可写:", err)
	} else {
		name := probe.Name()
		_ = probe.Close()
		_ = os.Remove(name)
		fmt.Println("  可写 ✓")
	}
	if home != "" {
		logPath := filepath.Join(home, "shellink.log")
		if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err != nil {
			fmt.Printf("日志文件: %s\n  打不开: %v\n", logPath, err)
		} else {
			_ = file.Close()
			fmt.Printf("日志文件: %s 可写 ✓\n", logPath)
		}
	}
	if listener, err := net.Listen("tcp", "127.0.0.1:7070"); err != nil {
		fmt.Println("端口 7070: 已被占用 (", err, ")")
	} else {
		_ = listener.Close()
		fmt.Println("端口 7070: 空闲 ✓")
	}
	if pids := runningShellink(); len(pids) > 0 {
		fmt.Println("残留进程: PID", joinPIDs(pids))
	} else {
		fmt.Println("残留进程: 无 ✓")
	}
	if tempBlocked {
		fmt.Println()
		fmt.Println("修复办法（任选其一，然后重新双击本启动器）:")
		fmt.Printf("  1) 重置文件夹完整性标签: icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T\n", dir)
		fmt.Println("  2) 把整个文件夹复制到普通目录（例如 E:\\Shellink）再运行")
	}
	fmt.Println("--------------")
}

// dataHome mirrors the preference order used by the daemon so the self-check
// names the directory that the start actually tried to use.
func dataHome() string {
	if explicit := strings.TrimSpace(os.Getenv("SHELLINK_HOME")); explicit != "" {
		return explicit
	}
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, "Shellink")
	}
	if base, err := os.UserConfigDir(); err == nil {
		return filepath.Join(base, "Shellink")
	}
	return ""
}

// writeDiagnostics drops everything a failed start knows into one file, so the
// problem can be diagnosed without asking the user to scroll a console window
// (the interesting lines are always the ones that scrolled away).
func writeDiagnostics(core, dir string, cause error, stdout, stderr string) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("time: %s", time.Now().Format("2006-01-02 15:04:05"))
	line("launcher: %s", os.Args[0])
	line("core: %s", core)
	line("cwd: %s", dir)
	line("SHELLINK_HOME: %s", orNone(os.Getenv("SHELLINK_HOME")))
	line("APPDATA: %s", orNone(os.Getenv("APPDATA")))
	line("TEMP: %s", orNone(os.Getenv("TEMP")))
	line("shellink processes: %s", joinPIDs(runningShellink()))
	line("error: %v", cause)
	line("")
	line("--- stderr ---")
	line("%s", strings.TrimSpace(stderr))
	line("")
	line("--- stdout ---")
	line("%s", strings.TrimSpace(stdout))
	line("")
	line("--- log tail ---")
	line("%s", logTail(core, dir))
	line("")
	line("--- hint ---")
	line("%s", hintFor(stderr+stdout))
	// Prefer the folder next to the binaries: it is easy to find, and when the
	// user profile is the thing that cannot be written, %TEMP% is unavailable
	// too.
	for _, path := range []string{
		filepath.Join(dir, "shellink-launcher-error.txt"),
		filepath.Join(os.TempDir(), "shellink-launcher-error.txt"),
	} {
		if err := os.WriteFile(path, []byte(b.String()), 0600); err == nil {
			return path
		}
	}
	return ""
}

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(unset)"
	}
	return value
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
	case strings.Contains(lower, "low integrity"):
		return "程序所在文件夹被标成了「低完整性(Low)」——浏览器下载或解压出来的文件常见这样，Windows 就禁止它写用户目录。按「自检」里的 icacls 命令重置标签，或把整个文件夹复制到普通目录再双击。"
	case strings.Contains(lower, "logging disabled"):
		return "数据目录里的日志文件写不进去，连临时目录也写不进去（上方有完整路径）：基本是安全软件（360/火绒/Defender 受控文件夹）挡住了这个进程，或被上一个进程占着。关掉防护试一次，或用 SHELLINK_HOME 指到一个可写目录再启动。"
	case strings.Contains(lower, "log file") && strings.Contains(lower, "not writable"):
		return "日志文件写不进去，已自动改写到临时目录（上方有完整路径）：多半是杀毒软件锁着数据目录，建议把数据目录加入信任列表。"
	case strings.Contains(lower, "address already in use"),
		strings.Contains(lower, "only one usage of each socket address"):
		return "端口已被占用：先运行 shellink.exe server stop，或在任务管理器结束残留的 shellink.exe 后再启动。"
	case strings.Contains(lower, "being used by another process"),
		strings.Contains(lower, "sharing violation"):
		return "文件被占用：上一次的 Shellink 进程（或杀毒软件）还握着数据库与日志，结束残留的 shellink.exe 后重试。"
	case strings.Contains(lower, "data directory") && strings.Contains(lower, "not writable"):
		return "数据目录不可写（上方有完整路径）：把这个目录加进杀毒软件的信任列表，或设置 SHELLINK_HOME 指到一个可写目录（例如 D:\\ShellinkData）再启动。"
	case strings.Contains(lower, "access is denied"),
		strings.Contains(lower, "permission denied"):
		return "权限被拒绝（上方有具体文件）：确认杀毒软件没有锁住 %APPDATA%\\Shellink，或用 SHELLINK_HOME 指向一个可写目录。"
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
