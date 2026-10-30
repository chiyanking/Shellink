package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// A checkResult is one line of the environment report.
//
// The report exists because Shellink is handed out as a folder: whoever unpacks
// it may lack the OpenSSH client, may be on a Windows release without ConPTY,
// or may have dropped it in a folder that cannot be written. Each of those
// fails later with a message that names a symptom rather than the cause, so the
// checks run before anything starts and name what is missing.
type checkResult struct {
	label  string
	status string
	detail string
	fix    string
	// reportOnly keeps a line out of the startup warnings: a taken port is
	// what the launcher is about to clear, and running elevated is a property
	// of the machine rather than a defect, so neither deserves a warning on
	// every double-click. Both still appear in the full --check report.
	reportOnly bool
}

const (
	statusOK   = "ok"
	statusWarn = "warn"
	statusFail = "fail"
)

// minConPTYBuild is Windows 10 version 1809, the first release with ConPTY.
// Older releases cannot host the command sessions at all.
const minConPTYBuild = 17763

// minFreeBytes leaves room for the database, the log and terminal output.
// Running out of disk shows up as a corrupted SQLite write, so it is worth a
// warning before the first session.
const minFreeBytes = 200 << 20

func collectChecks(dir string) []checkResult {
	var results []checkResult

	// 1. Operating system and the version that decides terminal support.
	label, build := osVersion()
	systemDetail := fmt.Sprintf("%s · %s/%s", label, runtime.GOOS, runtime.GOARCH)
	if build > 0 && build < minConPTYBuild {
		results = append(results, checkResult{
			label:  "操作系统",
			status: statusFail,
			detail: fmt.Sprintf("%s（内部版本 %d，过低）", systemDetail, build),
			fix:    "命令会话需要 Windows 10 1809（内部版本 17763）或更高版本，请升级系统。",
		})
	} else {
		results = append(results, checkResult{label: "操作系统", status: statusOK, detail: systemDetail})
	}

	// 2. Terminal backend. Split from the OS line so the failure says which
	//    capability is missing instead of just "old Windows".
	if runtime.GOOS == "windows" {
		if build == 0 {
			results = append(results, checkResult{label: "命令终端", status: statusWarn, detail: "无法判定系统内部版本（ConPTY 需要 17763 及以上）"})
		} else if build < minConPTYBuild {
			results = append(results, checkResult{
				label:  "命令终端",
				status: statusFail,
				detail: "ConPTY 不可用",
				fix:    "Windows 10 1809 及以上才带 ConPTY；低版本只能使用 SSH 会话。",
			})
		} else {
			results = append(results, checkResult{label: "命令终端", status: statusOK, detail: "ConPTY 可用"})
		}
	}

	// 3. The only external runtime dependency: the OpenSSH client. Command
	//    profiles work without it, SSH profiles do not.
	sshPath, sshVersion := findSSHClient()
	if sshPath == "" {
		results = append(results, checkResult{
			label:  "SSH 客户端",
			status: statusFail,
			detail: "未找到 ssh.exe",
			fix:    "设置 → 应用 → 可选功能 → 添加功能 → 勾选「OpenSSH 客户端」；或在管理员 PowerShell 执行: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0",
		})
	} else {
		results = append(results, checkResult{label: "SSH 客户端", status: statusOK, detail: fmt.Sprintf("%s (%s)", orUnknown(sshVersion), sshPath)})
	}

	// 4. Data directory: writable, and not blocked by a low integrity token.
	home := dataHome()
	switch {
	case home == "":
		results = append(results, checkResult{
			label:  "数据目录",
			status: statusFail,
			detail: "无法确定（SHELLINK_HOME 与 APPDATA 都为空）",
			fix:    "设置环境变量 SHELLINK_HOME 指向一个可写目录。",
		})
	case !dirWritable(home):
		fix := "把这个目录加入安全软件的信任列表，或用 SHELLINK_HOME 指向可写目录。"
		if lowIntegrityProcess() {
			fix = fmt.Sprintf("本进程连临时目录都写不了，说明程序文件夹被标成了「低完整性(Low)」：执行 icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T，或把整个文件夹复制到普通目录再运行。", dir)
		}
		results = append(results, checkResult{label: "数据目录", status: statusFail, detail: home + " 不可写", fix: fix})
	default:
		mode := ""
		if portableMode() {
			mode = "（便携模式，随文件夹移动）"
		}
		results = append(results, checkResult{label: "数据目录", status: statusOK, detail: home + mode})
	}

	// 5. Port. Occupied is only a warning here: it usually means a daemon of
	//    this very copy is still running, which the launcher stops next.
	if listener, err := net.Listen("tcp", "127.0.0.1:7070"); err != nil {
		results = append(results, checkResult{
			label:      "端口 7070",
			status:     statusWarn,
			detail:     "已被占用（若是残留的 Shellink 进程，启动时会自动结束）",
			fix:        "若提示来自别的程序，请关闭它；或结束残留的 shellink.exe 后重试。",
			reportOnly: true,
		})
	} else {
		_ = listener.Close()
		results = append(results, checkResult{label: "端口 7070", status: statusOK, detail: "空闲"})
	}

	// 6. Disk space for the database and session output.
	if free, ok := freeDiskBytes(dir); ok {
		if free < minFreeBytes {
			results = append(results, checkResult{
				label:  "磁盘空间",
				status: statusWarn,
				detail: fmt.Sprintf("剩余 %s，低于 %s", humanBytes(free), humanBytes(minFreeBytes)),
				fix:    "清理磁盘后重试；空间不足时数据库写入可能失败。",
			})
		} else {
			results = append(results, checkResult{label: "磁盘空间", status: statusOK, detail: "剩余 " + humanBytes(free)})
		}
	}

	// 7. The launcher automates stopping leftovers with these two tools.
	var missing []string
	for _, tool := range []string{"tasklist", "taskkill"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		results = append(results, checkResult{
			label:  "系统工具",
			status: statusWarn,
			detail: "缺少 " + strings.Join(missing, "、") + "（无法自动结束旧服务）",
			fix:    "程序仍可运行，但重复启动前需要手动在任务管理器结束 shellink.exe。",
		})
	}

	// 8. Integrity level and privileges. A low integrity process cannot hand
	//    the page to the browser at all - Windows keeps those processes away
	//    from the desktop - and an elevated launcher makes the data files
	//    administrator-owned and the browser elevated. Both are reported.
	if rid := integrityRID(); rid != 0 && rid < integrityMedium {
		results = append(results, checkResult{
			label:  "运行权限",
			status: statusFail,
			detail: "低完整性(Low)：能启动服务，但无法自动打开浏览器",
			fix:    fmt.Sprintf("执行 icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T 后重新双击；这个标签通常继承自上级目录（浏览器下载/解压到被标记的文件夹时常见）。", dir),
		})
	} else if isElevated() {
		results = append(results, checkResult{
			label:      "运行权限",
			status:     statusWarn,
			detail:     "管理员权限",
			fix:        "建议直接双击（不要右键“以管理员身份运行”）：数据文件会变成管理员所有，浏览器也会被提权拉起。",
			reportOnly: true,
		})
	} else {
		results = append(results, checkResult{label: "运行权限", status: statusOK, detail: "普通权限"})
	}

	return results
}

func orUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "版本未知"
	}
	return value
}

func humanBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, name := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, name)
		}
	}
	return fmt.Sprintf("%.1f PB", value)
}

// printEnvReport is the full table, used by "Shellink启动器.exe --check" and
// before a failure is reported.
func printEnvReport(dir string) int {
	results := collectChecks(dir)
	fmt.Println()
	fmt.Println("---- 环境检查 ----")
	printChecks(results)
	fmt.Println("------------------")
	failures := 0
	for _, result := range results {
		if result.status == statusFail {
			failures++
		}
	}
	if failures == 0 {
		fmt.Println("结论: 环境满足运行要求")
	} else {
		fmt.Printf("结论: 有 %d 项缺失，上方的 → 后面是解决办法\n", failures)
	}
	return failures
}

// printChecks renders the lines only, so the same checks can appear inside the
// failure self-check without a second, differently worded report.
func printChecks(results []checkResult) {
	for _, result := range results {
		mark := "✓"
		switch result.status {
		case statusWarn:
			mark = "!"
		case statusFail:
			mark = "✗"
		}
		fmt.Printf("%s %s %s\n", mark, padLabel(result.label+":", 12), result.detail)
		if result.fix != "" && result.status != statusOK {
			fmt.Println("    →", result.fix)
		}
	}
}

// printEnvWarnings keeps a normal start quiet: only the problems are shown, so
// a healthy double-click does not scroll a report the user has to read.
func printEnvWarnings(dir string) {
	results := collectChecks(dir)
	var problems []checkResult
	for _, result := range results {
		if result.status != statusOK && !result.reportOnly {
			problems = append(problems, result)
		}
	}
	if len(problems) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("环境检查发现以下问题:")
	for _, result := range problems {
		mark := "!"
		if result.status == statusFail {
			mark = "✗"
		}
		fmt.Printf("  %s %s %s\n", mark, padLabel(result.label+":", 12), result.detail)
		if result.fix != "" {
			fmt.Println("    →", result.fix)
		}
	}
	fmt.Println("  完整报告: Shellink启动器.exe --check")
	fmt.Println()
}

// padLabel pads a label to a fixed terminal width. Measuring bytes or runes
// both misalign a mixed Chinese/ASCII report, because a Chinese character takes
// two columns.
func padLabel(label string, width int) string {
	padding := width - displayWidth(label)
	if padding <= 0 {
		return label
	}
	return label + strings.Repeat(" ", padding)
}

func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		if isWideRune(r) {
			width += 2
			continue
		}
		width++
	}
	return width
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0xA4CF, // CJK radicals, kana, CJK ideographs
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x3FFFD: // CJK extension planes
		return true
	}
	return false
}

// dataHome mirrors the preference order used by the daemon so the checks and
// the self-check name the directory that the start actually tries to use.
func dataHome() string {
	if explicit := strings.TrimSpace(os.Getenv("SHELLINK_HOME")); explicit != "" {
		return explicit
	}
	if dir := executableDir(); dir != "" && portableMode() {
		return filepath.Join(dir, "shellink-data")
	}
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, "Shellink")
	}
	if base, err := os.UserConfigDir(); err == nil {
		return filepath.Join(base, "Shellink")
	}
	return ""
}

// portableMarker is the file that switches a folder into portable mode; it must
// match config.PortableMarker, because the daemon decides the data directory.
const portableMarker = "shellink.portable"

func executableDir() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(executable)
}

// portableMode reports whether the folder holding the binaries carries the
// portable marker, which is how the distributed archive keeps its data inside
// the folder instead of in the user profile.
func portableMode() bool {
	dir := executableDir()
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, portableMarker))
	return err == nil && !info.IsDir()
}

func dirWritable(path string) bool {
	if err := os.MkdirAll(path, 0700); err != nil {
		return false
	}
	probe, err := os.CreateTemp(path, ".shellink-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

// lowIntegrityProcess reports whether this process cannot write even the
// temporary directory, which on Windows means it runs with a low integrity
// token (an executable sitting in a folder extracted from a download).
func lowIntegrityProcess() bool {
	probe, err := os.CreateTemp("", "shellink-write-*")
	if err != nil {
		return true
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return false
}
