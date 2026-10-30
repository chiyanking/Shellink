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

	// "--open-url <url>" exercises only the browser launch path, so the part
	// that depends on privileges can be checked without touching the daemon.
	if len(os.Args) >= 3 && strings.EqualFold(strings.TrimLeft(os.Args[1], "-/"), "open-url") {
		method, err := openBrowser(os.Args[2])
		if err != nil {
			fmt.Println("打开失败:", err)
			os.Exit(1)
		}
		fmt.Println("已请求浏览器打开:", os.Args[2])
		fmt.Println("方式:", method)
		return
	}

	// "--check" answers "will this folder run on this machine?" without starting
	// anything: whatever was unpacked here may lack the SSH client, may sit on a
	// Windows release without ConPTY, or may live in a folder that cannot be
	// written. Exit code is the number of missing pieces, so the check is usable
	// from a script as well.
	for _, arg := range os.Args[1:] {
		switch strings.ToLower(strings.TrimLeft(arg, "-/")) {
		case "check", "env", "doctor":
			if failures := printEnvReport(dir); failures > 0 {
				os.Exit(failures)
			}
			return
		}
	}

	if _, err := os.Stat(core); err != nil {
		fmt.Println("未找到 shellink.exe，请将其与本启动器放在同一目录下。")
		fmt.Println("目录:", dir)
		pause()
		return
	}

	// Only the problems are reported here; a healthy start stays quiet.
	printEnvWarnings(dir)

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
	opened := false
	if os.Getenv("SHELLINK_NO_BROWSER") == "" {
		// A file that carries a low mandatory label - typical for a folder that
		// was extracted under a marked parent - makes this process low
		// integrity, and Windows keeps such a process away from the desktop:
		// every way of handing over the URL fails silently while the service
		// itself runs fine. That is worth a fix line instead of a promise.
		if rid := integrityRID(); rid != 0 && rid < integrityMedium {
			fmt.Println()
			fmt.Println("提示: 本程序运行在「低完整性(Low)」，Windows 不允许它把页面交给浏览器，")
			fmt.Println("      所以浏览器不会自动弹出（服务本身已正常启动）。执行一次下面这条命令即可修复:")
			fmt.Printf("      icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T\n", dir)
			fmt.Println("      然后重新双击本程序。这个标签一般继承自上级目录，重置上级目录才一劳永逸。")
		} else if !isElevated() && elevatedBrowser() {
			// A browser running with administrator rights rejects a URL from a
			// process that has less: Edge answers "现有实例正在以提升的权限
			// 运行" and offers to restart itself. The launcher hands the URL to
			// the browser with the shell's rights, but it cannot reach a
			// browser running higher than the shell, so say what to do instead
			// of making a modal dialog pop up.
			fmt.Println()
			fmt.Println("提示: 现有浏览器以管理员权限运行，普通权限的启动器无法把页面交给它")
			fmt.Println("      （Edge 会弹“现有实例正在以提升的权限运行”，选“是”会把浏览器关掉重开）。")
			fmt.Println("      请先关闭这些浏览器窗口，或重启一次浏览器后再打开下面的地址。")
		} else if _, err := openBrowser(url); err != nil {
			fmt.Println()
			fmt.Println("提示: 没能自动打开浏览器，请手动访问下面的地址。")
			fmt.Println("      原因:", err)
		} else {
			opened = true
		}
	}

	fmt.Println()
	fmt.Println("──────────────────────────────────────────")
	if opened {
		fmt.Println("Web 控制台已打开:", url)
	} else {
		fmt.Println("Web 控制台地址:", url)
	}
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
	if !isElevated() {
		// An elevated daemon cannot be killed by a normal process, and taskkill
		// has no way to report that usefully, so name the reason here.
		fmt.Println("  提示: 该进程以管理员权限运行，普通窗口无法结束它 —— 请在任务管理器中结束，")
		fmt.Println("        或先关闭所有 Shellink 窗口再重新启动本程序。")
	}
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
	// The shared checks already cover the data directory, the port, disk space
	// and the process integrity. The two lines below only make sense for a
	// start that has just failed.
	printChecks(collectChecks(dir))
	home := dataHome()
	if home != "" {
		logPath := filepath.Join(home, "shellink.log")
		if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err != nil {
			fmt.Println("! " + padLabel("日志文件:", 12) + logPath)
			fmt.Println("    → 打不开:", err)
		} else {
			_ = file.Close()
			fmt.Println("✓ " + padLabel("日志文件:", 12) + logPath + " 可写")
		}
	}
	if pids := runningShellink(); len(pids) > 0 {
		fmt.Println("! " + padLabel("残留进程:", 12) + "PID " + joinPIDs(pids))
	} else {
		fmt.Println("✓ " + padLabel("残留进程:", 12) + "无")
	}
	if lowIntegrityProcess() {
		fmt.Println()
		fmt.Println("修复办法（任选其一，然后重新双击本启动器）:")
		fmt.Printf("  1) 重置文件夹完整性标签: icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T\n", dir)
		fmt.Println("  2) 把整个文件夹复制到普通目录（例如 E:\\Shellink）再运行")
	}
	fmt.Println("--------------")
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

func pause() {
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
}
