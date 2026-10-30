package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shellink/internal/app"
	"shellink/internal/client"
	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/httpapi"
	"shellink/internal/protocol"
	"shellink/internal/rpc"
)

type Arguments struct {
	Words []string
	Flags map[string]string
	Set   map[string]bool
}

func Parse(args []string) Arguments {
	result := Arguments{Flags: make(map[string]string), Set: make(map[string]bool)}
	boolean := map[string]bool{"json": true, "help": true, "detach": true, "no-newline": true, "include-internal": true, "check": true, "yes": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" {
			result.Set["help"] = true
			result.Flags["help"] = "true"
			continue
		}
		if arg == "-V" {
			result.Set["version"] = true
			result.Flags["version"] = "true"
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			result.Words = append(result.Words, arg)
			continue
		}
		keyValue := strings.TrimPrefix(arg, "--")
		key, value, hasValue := strings.Cut(keyValue, "=")
		if hasValue {
			result.Flags[key] = value
			result.Set[key] = true
			continue
		}
		result.Set[key] = true
		// "--color" is the one flag whose value is optional: bare --color means
		// "always", while --color=never and --color never both make sense.
		if key == "color" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") && isColorMode(args[i+1]) {
				i++
				result.Flags["color"] = strings.ToLower(args[i])
			} else {
				result.Flags["color"] = "always"
			}
			continue
		}
		if boolean[key] {
			result.Flags[key] = "true"
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			i++
			result.Flags[key] = args[i]
		} else {
			result.Flags[key] = "true"
		}
	}
	if value, ok := result.Flags["version"]; ok && value != "true" {
		result.Set["version"] = true
	}
	return result
}

func Main(args []string) error {
	parsed := Parse(args)
	words := parsed.Words
	flags := parsed.Flags
	applyColorPreference(flags)
	group, action := word(words, 0), word(words, 1)
	if parsed.Set["version"] || group == "version" {
		if flagBool(flags, "json") {
			printOutput(map[string]any{"name": "shellink", "version": rpc.Version, "commit": rpc.Commit, "protocolVersion": rpc.ProtocolVersion}, true)
		} else {
			fmt.Printf("%s %s  %s\n", strongText("Shellink"), cyan(rpc.Version), dim("commit "+rpc.Commit+"  \u00b7  protocol "+strconv.Itoa(rpc.ProtocolVersion)))
		}
		return nil
	}
	if parsed.Set["help"] || group == "help" || group == "" {
		// "shellink help session" asks for the topic named after the word.
		topic := groupOrDefault(group)
		if group == "help" {
			topic = groupOrDefault(action)
		}
		fmt.Print(help(topic))
		return nil
	}
	if group == "agent-doc" {
		if flagBool(flags, "json") {
			printOutput(httpapi.AgentDocJSON, true)
		} else {
			fmt.Print(httpapi.AgentDoc)
		}
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if group == "server" {
		return handleServer(cfg, action, flags)
	}
	if group == "upgrade" {
		return handleUpgrade(cfg, flags)
	}
	if group == "cli" {
		return runInteractive(cfg)
	}
	if group == "mcp" {
		return handleMCP(cfg, flags)
	}
	if group != "profile" && group != "session" && group != "webhook" {
		return fmt.Errorf("unknown command: %s", group)
	}
	connection, err := daemon.Ensure(cfg, 10*time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	return handleRPC(connection, words, flags)
}

func handleServer(cfg config.Config, action string, flags map[string]string) error {
	switch action {
	case "run":
		return runServer(cfg)
	case "logs":
		lines, _ := parseInt(flags["lines"], 40)
		fmt.Println(daemon.LogTail(cfg.LogPath, int(lines)))
		return nil
	case "status", "stop":
		current, err := daemon.Connect(cfg, time.Second)
		if err != nil {
			if action == "status" {
				printOutput(map[string]any{"running": false}, flagBool(flags, "json"))
			} else {
				printOutput(map[string]any{"stopping": false, "running": false}, flagBool(flags, "json"))
			}
			return nil
		}
		defer current.Close()
		method := "system.status"
		if action == "stop" {
			method = "system.stop"
		}
		value, err := current.Request(method, protocol.Object(), 30*time.Second)
		if err == nil {
			printOutput(protocol.ToJSONValue(value), flagBool(flags, "json"))
		}
		return err
	case "start", "restart":
		// Fail with the folder name when the data directory cannot be written:
		// the daemon's own complaint is written to a log file that may be the
		// very thing that is not writable.
		if err := config.CheckDataHome(cfg.DataHome); err != nil {
			return err
		}
		if action == "restart" {
			if current, err := daemon.Connect(cfg, time.Second); err == nil {
				_, _ = current.Request("system.stop", protocol.Object(), time.Second)
				current.Close()
				time.Sleep(300 * time.Millisecond)
			}
		}
		current, err := daemon.Ensure(cfg, 10*time.Second)
		if err != nil {
			return err
		}
		// The instance that answers here may be the one that is still shutting
		// down (the user pressed Stop a moment ago): it accepts the connection
		// and then drops the request. Wait for it to go away and bring up a
		// fresh daemon instead of exiting non-zero over a half-closed pipe.
		value, err := current.Request("system.status", protocol.Object(), 5*time.Second)
		if err != nil {
			current.Close()
			time.Sleep(time.Second)
			if retry, retryErr := daemon.Ensure(cfg, 10*time.Second); retryErr == nil {
				current = retry
				value, err = current.Request("system.status", protocol.Object(), 5*time.Second)
			}
		}
		defer current.Close()
		if err == nil {
			printOutput(protocol.ToJSONValue(value), flagBool(flags, "json"))
		}
		return err
	default:
		return fmt.Errorf("usage: shellink server start|status|stop|restart|logs|run")
	}
}

func runServer(cfg config.Config) error {
	runtime := app.New(cfg)
	if err := runtime.Start(); err != nil {
		return err
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, platformSignals()...)
	defer signal.Stop(signals)
	select {
	case <-runtime.Done():
	case <-signals:
	case err := <-runtime.ServeErrors():
		_ = runtime.Close()
		return err
	}
	return runtime.Close()
}

func handleRPC(connection *client.Client, words []string, flags map[string]string) error {
	group, action, id := word(words, 0), word(words, 1), word(words, 2)
	jsonOutput := flagBool(flags, "json")
	request := func(method string, params map[string]any, timeout time.Duration) (protocol.Value, error) {
		if params == nil {
			params = map[string]any{}
		}
		value, err := connection.Request(method, protocol.JSONValue(params), timeout)
		if err == nil {
			printOutput(protocol.ToJSONValue(value), jsonOutput)
		}
		return value, err
	}
	if group == "profile" {
		switch action {
		case "list":
			return requestOnlyError(request("profiles.list", map[string]any{"q": option(flags, "query", "q")}, 0))
		case "get":
			return requestOnlyError(request("profiles.get", map[string]any{"id": required(id, "profile ID")}, 0))
		case "create":
			body, err := readInput(required(flags["input"], "--input"))
			if err != nil {
				return err
			}
			return requestOnlyError(request("profiles.create", body, 0))
		case "update":
			body, err := readInput(required(flags["input"], "--input"))
			if err != nil {
				return err
			}
			return requestOnlyError(request("profiles.update", map[string]any{"id": required(id, "profile ID"), "profile": body}, 0))
		case "delete":
			return requestOnlyError(request("profiles.delete", map[string]any{"id": required(id, "profile ID")}, 0))
		}
		return fmt.Errorf("usage: shellink profile list|get|create|update|delete")
	}
	if group == "webhook" {
		switch action {
		case "list":
			return requestOnlyError(request("webhooks.list", nil, 0))
		case "create":
			body, err := readInput(required(flags["input"], "--input"))
			if err != nil {
				return err
			}
			return requestOnlyError(request("webhooks.create", body, 0))
		case "delete":
			return requestOnlyError(request("webhooks.delete", map[string]any{"id": required(id, "webhook ID")}, 0))
		}
		return fmt.Errorf("usage: shellink webhook list|create|delete")
	}
	if group != "session" {
		return fmt.Errorf("unknown command: %s", group)
	}
	sid := required(id, "session ID")
	switch action {
	case "list":
		return requestOnlyError(request("sessions.list", nil, 0))
	case "create":
		cols, err := optionalInt(flags, "cols")
		if err != nil {
			return err
		}
		rows, err := optionalInt(flags, "rows")
		if err != nil {
			return err
		}
		return requestOnlyError(request("sessions.create", map[string]any{"profileId": required(flags["profile"], "--profile"), "cols": cols, "rows": rows}, 0))
	case "state":
		return requestOnlyError(request("sessions.state", map[string]any{"id": sid}, 0))
	case "history":
		since, err := parseInt(flags["since"], 0)
		if err != nil {
			return err
		}
		limit, err := parseInt(flags["limit"], 2000)
		if err != nil {
			return err
		}
		// Ask the daemon for the colours it recorded only when this stdout can
		// actually show them; otherwise the transcript would arrive full of
		// escape sequences that render as garbage in a file or a pipe, and
		// --json in particular must stay parseable.
		return requestOnlyError(request("sessions.history", map[string]any{
			"id":              sid,
			"since":           since,
			"limit":           limit,
			"includeInternal": flagBool(flags, "include-internal"),
			"color":           useColor() && !flagBool(flags, "json"),
		}, 0))
	case "input":
		return requestOnlyError(request("sessions.input", map[string]any{"id": sid, "text": required(flags["text"], "--text"), "appendNewline": !flagBool(flags, "no-newline")}, 0))
	case "exec":
		command := required(flags["command"], "--command")
		timeoutMS, err := parseInt(flags["timeout"], 20_000)
		if err != nil {
			return err
		}
		method := "sessions.exec"
		wait := time.Duration(timeoutMS+1000) * time.Millisecond
		if flagBool(flags, "detach") {
			method, wait = "sessions.execStart", 30*time.Second
		}
		return requestOnlyError(request(method, map[string]any{"id": sid, "command": command, "timeoutMs": timeoutMS}, wait))
	case "exec-status":
		jobID := flags["job"]
		if jobID == "" {
			jobID = required(id, "job ID (positional) or --job; a session ID is rejected by the daemon with Job not found")
		}
		since, err := parseInt(flags["since"], 0)
		if err != nil {
			return err
		}
		waitMS, err := parseInt(flags["wait"], 0)
		if err != nil {
			return err
		}
		return requestOnlyError(request("sessions.execStatus", map[string]any{"jobId": jobID, "since": since, "waitMs": waitMS}, time.Duration(waitMS+1000)*time.Millisecond))
	case "exec-cancel":
		jobID := flags["job"]
		if jobID == "" {
			jobID = required(id, "job ID (positional) or --job")
		}
		return requestOnlyError(request("sessions.execCancel", map[string]any{"jobId": jobID}, 0))
	case "mode":
		return requestOnlyError(request("sessions.mode", map[string]any{"id": sid, "mode": required(flags["mode"], "--mode")}, 0))
	case "close":
		return requestOnlyError(request("sessions.close", map[string]any{"id": sid}, 0))
	case "remove-record":
		return requestOnlyError(request("sessions.removeRecord", map[string]any{"id": sid}, 0))
	case "upload":
		data, err := readBytes(required(flags["input"], "--input"))
		if err != nil {
			return err
		}
		method := "sessions.upload"
		requestTimeout := time.Duration(intOption(flags, "timeout", 120_000)+1000) * time.Millisecond
		if flagBool(flags, "detach") {
			method, requestTimeout = "sessions.uploadStart", 30*time.Second
		}
		return requestOnlyError(request(method, map[string]any{"id": sid, "path": required(flags["path"], "--path"), "data": data, "timeoutMs": intOption(flags, "timeout", 120_000), "sha256": flags["sha256"]}, requestTimeout))
	case "download":
		output := filepath.Clean(required(flags["output"], "--output"))
		if flagBool(flags, "detach") {
			params := map[string]any{"id": sid, "path": required(flags["path"], "--path"), "output": output, "timeoutMs": intOption(flags, "timeout", 120_000)}
			return requestOnlyError(request("sessions.downloadStart", params, 30*time.Second))
		}
		value, err := connection.Request("sessions.download", protocol.JSONValue(map[string]any{"id": sid, "path": required(flags["path"], "--path"), "timeoutMs": intOption(flags, "timeout", 120_000)}), time.Duration(intOption(flags, "timeout", 120_000)+1000)*time.Millisecond)
		if err != nil {
			return err
		}
		fields := protocol.MapValue(value)
		data, ok := fields["data"].([]byte)
		if !ok {
			return fmt.Errorf("download returned no file data")
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			return err
		}
		result := make(map[string]any, len(fields))
		for key, field := range fields {
			if key != "data" {
				result[key] = protocol.ToJSONValue(field)
			}
		}
		result["output"] = output
		printOutput(result, jsonOutput)
		return nil
	case "edit":
		body, err := readInput(required(flags["input"], "--input"))
		if err != nil {
			return err
		}
		body["id"] = sid
		method, requestTimeout := "sessions.edit", 30*time.Second
		if flagBool(flags, "detach") {
			method = "sessions.editStart"
		}
		return requestOnlyError(request(method, body, requestTimeout))
	default:
		return fmt.Errorf("usage: shellink session list|create|state|history|input|exec|exec-status|exec-cancel|upload|download|edit|mode|close|remove-record\nexec-status/exec-cancel take a job ID (positional or --job), not a session ID")
	}
}

func readInput(path string) (map[string]any, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("input JSON must be an object")
	}
	return value, nil
}

func readBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func printOutput(value any, jsonLine bool) {
	if value == nil {
		return
	}
	if text, ok := value.(string); ok {
		fmt.Println(text)
		return
	}
	if jsonLine {
		// Machine output must stay byte-for-byte what it always was: a single
		// line, no color, no table.
		if data, err := json.Marshal(value); err == nil {
			fmt.Println(string(data))
		}
		return
	}
	fmt.Println(render(value))
}

func requestOnlyError(_ protocol.Value, err error) error { return err }
func word(words []string, index int) string {
	if index < len(words) {
		return words[index]
	}
	return ""
}

// isColorMode reports whether the word names one of the three accepted
// settings, so `--color never` is not mistaken for a positional argument.
func isColorMode(value string) bool {
	switch strings.ToLower(value) {
	case "auto", "always", "never":
		return true
	}
	return false
}

// applyColorPreference lets one run override the environment, which is how a
// user asks for color while piping into `less -R` or turns it off for paste.
func applyColorPreference(flags map[string]string) {
	mode := strings.ToLower(flags["color"])
	if !isColorMode(mode) {
		return
	}
	ResetColorForTest()
	_ = os.Setenv("SHELLINK_COLOR", mode)
}

func groupOrDefault(value string) string {
	if value == "" {
		return "root"
	}
	return value
}
func required(value, name string) string {
	if value == "" {
		return "<missing " + name + ">"
	}
	return value
}
func option(flags map[string]string, first, second string) string {
	if value := flags[first]; value != "" {
		return value
	}
	return flags[second]
}
func flagBool(flags map[string]string, key string) bool {
	return flags[key] == "true" || flags[key] == "1"
}

func parseInt(value string, fallback int64) (int64, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func optionalInt(flags map[string]string, key string) (any, error) {
	if flags[key] == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(flags[key])
	if err != nil {
		return nil, fmt.Errorf("--%s must be an integer", key)
	}
	return value, nil
}

func intOption(flags map[string]string, key string, fallback int) int {
	if value, err := strconv.Atoi(flags[key]); err == nil && flags[key] != "" {
		return value
	}
	return fallback
}

// help renders one help page. The text is unchanged from before; what changed
// is that it now reads like a page instead of one long line: a bold title,
// aligned columns, dim options and a couple of real examples, which is the part
// people actually copy.
func help(topic string) string {
	switch topic {
	case "profile":
		return commandPage(
			"Profile", "Manage SSH and command profiles",
			"shellink profile list|get|create|update|delete",
			[]example{
				{"list", "shellink profile list"},
				{"list only the ones matching a word", "shellink profile list --query prod"},
				{"create from a JSON body", "shellink profile create --input profile.json"},
			},
			[]string{"Credentials are only ever read with --input FILE or from stdin.", "--input - reads the body from standard input."},
		)
	case "session":
		return commandPage(
			"Session", "Create, drive and inspect terminal sessions",
			"shellink session list|create|state|history|input|exec|exec-status|exec-cancel|upload|download|edit|mode|close|remove-record",
			[]example{
				{"list sessions", "shellink session list"},
				{"open one against a profile", "shellink session create --profile <profile-id>"},
				{"run a command in it", "shellink session exec --id <id> --command \"uptime\""},
				{"watch what it printed", "shellink session history --id <id> --limit 200"},
			},
			[]string{
				"File transfer needs AUTO mode: MANUAL means a human holds the terminal.",
				"--detach returns a job id immediately and runs the job in the background.",
				"exec-status/exec-cancel take a job ID (from exec --detach or upload/download --detach), not a session ID: shellink session exec-status '<job-id>'",
			},
		)
	case "server":
		return commandPage(
			"Server", "Manage the background daemon",
			"shellink server start|status|stop|restart|logs|run",
			[]example{
				{"start it in the background", "shellink server start"},
				{"see whether it is up", "shellink server status"},
				{"tail the log", "shellink server logs --lines 100"},
			},
			[]string{"start/stop/restart are used by the launcher; run serves one process in the foreground."},
		)
	case "webhook":
		return commandPage(
			"Webhook", "Manage event callbacks",
			"shellink webhook list|create|delete",
			[]example{
				{"list callbacks", "shellink webhook list"},
				{"add one", "shellink webhook create --input webhook.json"},
			},
			nil,
		)
	case "mcp":
		return commandPage(
			"MCP", "Speak the Model Context Protocol over stdio",
			"shellink mcp serve",
			[]example{{"let an agent drive Shellink", "shellink mcp serve"}},
			[]string{"One line of JSON per message on stdout; nothing else is printed there."},
		)
	case "upgrade":
		return commandPage(
			"Upgrade", "Replace a release binary with a newer one",
			"shellink upgrade [--check] [--yes]",
			[]example{
				{"only look for a newer release", "shellink upgrade --check"},
				{"download and replace", "shellink upgrade"},
			},
			nil,
		)
	}
	return rootHelp()
}

type example struct {
	line    string
	command string
}

// commandPage lays one topic out: what it is, how it is called, what to type,
// and the small print nobody reads until something fails.
func commandPage(title, summary, usage string, examples []example, notes []string) string {
	var out strings.Builder
	out.WriteString(strongText(title) + "  " + dim("\u2014") + "  " + summary + "\n\n")
	out.WriteString(headText("Usage") + "\n  " + strongText(usage) + "\n")
	if len(examples) > 0 {
		out.WriteString("\n" + headText("Examples") + "\n")
		width := 0
		for _, item := range examples {
			width = max(width, visibleWidth(item.line))
		}
		for _, item := range examples {
			out.WriteString(fmt.Sprintf("  %s  %s\n", padRightVisible(item.line, width), cyan(item.command)))
		}
	}
	if len(notes) > 0 {
		out.WriteString("\n" + headText("Notes") + "\n")
		for _, note := range notes {
			out.WriteString("  " + gray(symbolPointer) + " " + dim(note) + "\n")
		}
	}
	return out.String()
}

func rootHelp() string {
	var out strings.Builder
	out.WriteString(strongText("Shellink "+rpc.Version) + "  " + dim("commit "+rpc.Commit+"  \u00b7  protocol "+strconv.Itoa(rpc.ProtocolVersion)) + "\n")
	out.WriteString(dim("Terminal sessions that a human and an agent can share.\n"))
	out.WriteString("\n" + headText("Usage") + "\n  " + strongText("shellink <command> [options]") + "\n")
	commands := [][2]string{
		{"cli", "Interactive session console"},
		{"server", "Manage the daemon"},
		{"profile", "Manage connection profiles"},
		{"session", "Manage terminal sessions"},
		{"webhook", "Manage event callbacks"},
		{"mcp", "Serve the Model Context Protocol over stdio"},
		{"agent-doc", "Print AI agent documentation"},
		{"version", "Print version information"},
		{"upgrade", "Upgrade a release binary"},
	}
	out.WriteString("\n" + headText("Commands") + "\n")
	for _, item := range commands {
		out.WriteString("  " + cyan(padRightVisible(item[0], 12)) + item[1] + "\n")
	}
	options := [][2]string{
		{"--json", "One JSON object per line, for scripts"},
		{"--color auto|always|never", "Colorize the human readable form"},
		{"--help, -h", "Show this help"},
	}
	out.WriteString("\n" + headText("Global options") + "\n")
	for _, item := range options {
		out.WriteString("  " + yellow(padRightVisible(item[0], 26)) + dim(item[1]) + "\n")
	}
	out.WriteString("\n" + headText("Examples") + "\n")
	for _, item := range []example{
		{"list everything", "shellink session list"},
		{"machine readable list", "shellink session list --json"},
		{"one answer as an object", "shellink profile get <id>"},
		{"details for one command", "shellink help session"},
	} {
		out.WriteString("  " + cyan(item.command) + "  " + dim(item.line) + "\n")
	}
	out.WriteString("\n" + dim("Use shellink <command> --help for details.\n"))
	return out.String()
}
