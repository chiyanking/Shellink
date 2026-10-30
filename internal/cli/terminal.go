package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"shellink/internal/client"
	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/protocol"
)

type terminalDimensions struct{ cols, rows int }

func runInteractive(cfg config.Config) error {
	if !terminalInput(os.Stdin) || !terminalInput(os.Stdout) {
		return fmt.Errorf("interactive terminal required")
	}
	connection, err := daemon.Ensure(cfg, 10*time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	return runTUI(connection)
}

func runTUI(connection *client.Client) error {
	if !terminalInput(os.Stdin) || !terminalInput(os.Stdout) {
		return fmt.Errorf("interactive terminal required")
	}
	restore, err := enableRawTerminal()
	if err != nil {
		return err
	}
	defer restore()
	fmt.Fprint(os.Stdout, "\x1b[?25l\x1b[2J\x1b[H")
	defer fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[0m\r\n")

	input := make(chan []byte, 32)
	go readTerminalInput(input)

	dimensions := getTerminalDimensions()
	view := "sessions"
	items := []map[string]any{}
	selected := 0
	query := ""
	searching := false
	message := ""
	showHelp := false
	refresh := func() {
		method := "sessions.list"
		if view == "profiles" {
			method = "profiles.list"
		}
		value, requestErr := connection.Request(method, protocol.Object(), 15*time.Second)
		if requestErr != nil {
			message = requestErr.Error()
			return
		}
		all := protocol.ToJSONValue(value)
		rows, _ := all.([]any)
		items = items[:0]
		needle := strings.ToLower(query)
		for _, row := range rows {
			fields, ok := row.(map[string]any)
			if !ok {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(searchableRow(view, fields)), needle) {
				continue
			}
			items = append(items, fields)
		}
		if view == "sessions" {
			for i := 1; i < len(items); i++ {
				if items[i]["active"] == true && items[i-1]["active"] != true {
					items[i], items[i-1] = items[i-1], items[i]
				}
			}
		}
		if selected >= len(items) {
			selected = max(0, len(items)-1)
		}
		message = ""
	}
	refresh()
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		current := getTerminalDimensions()
		if current != dimensions {
			dimensions = current
		}
		drawDashboard(dimensions, view, items, selected, query, searching, message, showHelp)
		select {
		case <-ticker.C:
			if current := getTerminalDimensions(); current != dimensions {
				dimensions = current
			}
			if view == "sessions" {
				refresh()
			}
		case data, ok := <-input:
			if !ok {
				return nil
			}
			if data == nil {
				return nil
			}
			if searching {
				switch {
				case bytes.Contains(data, []byte{0x1b}), bytes.Contains(data, []byte{0x03}):
					searching = false
				case bytes.Contains(data, []byte{0x7f}), bytes.Contains(data, []byte{0x08}):
					runes := []rune(query)
					if len(runes) > 0 {
						query = string(runes[:len(runes)-1])
					}
				case bytes.Contains(data, []byte{'\r'}), bytes.Contains(data, []byte{'\n'}):
					searching = false
					selected = 0
				default:
					for _, r := range string(data) {
						if r >= ' ' && r != 0x7f {
							query += string(r)
						}
					}
				}
				refresh()
				continue
			}
			switch {
			case bytes.Contains(data, []byte{0x03}):
				return nil
			case bytes.Equal(data, []byte("1")), bytes.Equal(data, []byte("\t")), bytes.Contains(data, []byte("\x1b[D")):
				view, selected, query, showHelp = "sessions", 0, "", false
				refresh()
			case bytes.Equal(data, []byte("2")), bytes.Contains(data, []byte("\x1b[C")):
				view, selected, query, showHelp = "profiles", 0, "", false
				refresh()
			case bytes.Equal(data, []byte{0x1b}):
				nextView, exit := dashboardEscapeAction(view)
				if exit {
					return nil
				}
				view, selected, query, showHelp = nextView, 0, "", false
				refresh()
			case bytes.Equal(data, []byte("j")), bytes.Contains(data, []byte("\x1b[B")):
				if len(items) > 0 {
					selected = (selected + 1) % len(items)
				}
			case bytes.Equal(data, []byte("k")), bytes.Contains(data, []byte("\x1b[A")):
				if len(items) > 0 {
					selected = (selected - 1 + len(items)) % len(items)
				}
			case bytes.Equal(data, []byte("r")):
				refresh()
			case bytes.Equal(data, []byte("/")):
				searching = true
				query = ""
			case bytes.Equal(data, []byte("?")):
				showHelp = !showHelp
			case bytes.Equal(data, []byte("\r")), bytes.Equal(data, []byte("\n")):
				if selected >= 0 && selected < len(items) {
					if view == "profiles" {
						profileID, _ := items[selected]["id"].(string)
						value, requestErr := connection.Request("sessions.create", protocol.JSONValue(map[string]any{"profileId": profileID, "cols": dimensions.cols, "rows": max(5, dimensions.rows-3)}), 30*time.Second)
						if requestErr != nil {
							message = requestErr.Error()
							continue
						}
						id, _ := protocol.StringValue(protocol.MapValue(value)["id"])
						if sessionErr := runSessionTerminal(connection, input, ticker.C, id, dimensions); sessionErr != nil {
							message = sessionErr.Error()
						}
					} else {
						if items[selected]["active"] != true {
							message = "Session is disconnected and cannot be opened."
							continue
						}
						id, _ := items[selected]["id"].(string)
						if sessionErr := runSessionTerminal(connection, input, ticker.C, id, dimensions); sessionErr != nil {
							message = sessionErr.Error()
						}
					}
					refresh()
				}
			}
		}
	}
}

func runSessionTerminal(connection *client.Client, input <-chan []byte, resizePoll <-chan time.Time, sessionID string, dimensions terminalDimensions) error {
	value, err := connection.Request("events.subscribe", protocol.JSONValue(map[string]any{"sessionId": sessionID, "replay": true}), 10*time.Second)
	if err != nil {
		return err
	}
	fields := protocol.MapValue(value)
	subscriptionID, _ := protocol.StringValue(fields["subscriptionId"])
	initial := protocol.MapValue(fields["initial"])
	state := protocol.MapValue(initial["state"])
	if active, ok := protocol.BoolValue(state["active"]); ok && !active {
		unsubscribeEvents(connection, subscriptionID)
		return fmt.Errorf("session is disconnected")
	}
	mode, _ := protocol.StringValue(state["mode"])
	sessionState, _ := protocol.StringValue(state["state"])
	manual := mode == "MANUAL"
	tookControl := false
	defer func() {
		if tookControl {
			_, _ = connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "AUTO"}), time.Second)
		}
		unsubscribeEvents(connection, subscriptionID)
	}()
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
	defer fmt.Fprint(os.Stdout, "\x1b[?25l")
	if replay, ok := protocol.StringValue(initial["replay"]); ok {
		fmt.Fprint(os.Stdout, replay)
	}
	drawSessionStatus(sessionID, manual, dimensions)
	setSessionCursor(manual, sessionState)
	_ = resizeSession(connection, sessionID, dimensions)
	for {
		select {
		case <-resizePoll:
			if current := getTerminalDimensions(); current != dimensions {
				dimensions = current
				_ = resizeSession(connection, sessionID, dimensions)
				drawSessionStatus(sessionID, manual, dimensions)
			}
		case event := <-connection.Events():
			eventFields := protocol.MapValue(event)
			if eventFields["subscriptionId"] != subscriptionID {
				continue
			}
			eventName, _ := protocol.StringValue(eventFields["event"])
			data := protocol.MapValue(eventFields["data"])
			switch eventName {
			case "session.data":
				if raw, ok := protocol.StringValue(data["raw"]); ok {
					_, _ = fmt.Fprint(os.Stdout, raw)
				}
			case "session.mode":
				mode, _ = protocol.StringValue(data["mode"])
				manual = mode == "MANUAL"
				drawSessionStatus(sessionID, manual, dimensions)
				setSessionCursor(manual, sessionState)
			case "session.state":
				nextState, _ := protocol.StringValue(data["state"])
				sessionState = nextState
				setSessionCursor(manual, sessionState)
			case "session.closed":
				reason, _ := protocol.StringValue(data["reason"])
				fmt.Fprintf(os.Stdout, "\r\n%s session closed: %s\r\n", gray("[shellink]"), red(reason))
			}
		case data, ok := <-input:
			if !ok || data == nil {
				return nil
			}
			if isSessionReturnKey(data) {
				return nil
			}
			if !manual && bytes.Equal(data, []byte("m")) {
				if _, err := connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "MANUAL"}), 5*time.Second); err != nil {
					fmt.Fprintf(os.Stdout, "\r\n%s %s\r\n", gray("[shellink]"), failSymbol()+" "+red(err.Error()))
				} else {
					manual, tookControl = true, true
					drawSessionStatus(sessionID, manual, dimensions)
					setSessionCursor(manual, sessionState)
				}
				continue
			}
			if manual && (bytes.Equal(data, []byte{0x1c}) || bytes.Contains(data, []byte("\x1b[92;5u"))) {
				if _, err := connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "AUTO"}), 5*time.Second); err == nil {
					manual, tookControl = false, false
					drawSessionStatus(sessionID, manual, dimensions)
					setSessionCursor(manual, sessionState)
				}
				continue
			}
			if manual {
				_, err := connection.Request("sessions.input", protocol.JSONValue(map[string]any{"id": sessionID, "text": string(data), "appendNewline": false, "manual": true}), 5*time.Second)
				if err != nil {
					fmt.Fprintf(os.Stdout, "\r\n%s %s\r\n", gray("[shellink]"), failSymbol()+" "+red(err.Error()))
				}
			}
		}
	}
}

func unsubscribeEvents(connection *client.Client, id string) {
	if id != "" {
		_, _ = connection.Request("events.unsubscribe", protocol.JSONValue(map[string]any{"subscriptionId": id}), 3*time.Second)
	}
}

func resizeSession(connection *client.Client, id string, dimensions terminalDimensions) error {
	_, err := connection.Request("sessions.resize", protocol.JSONValue(map[string]any{"id": id, "cols": dimensions.cols, "rows": max(5, dimensions.rows-3)}), 3*time.Second)
	return err
}

// drawSessionStatus keeps a one-line bar at the bottom of the screen. The
// session id and the mode are the two things worth a color there: "MANUAL"
// means a human has the terminal and nothing automatic will get through.
func drawSessionStatus(sessionID string, manual bool, dimensions terminalDimensions) {
	cols, rows := max(1, dimensions.cols), max(1, dimensions.rows)
	mode := cyan("AUTO") + gray(" (read only)")
	if manual {
		mode = modeText("MANUAL")
	}
	status := strings.Join([]string{
		gray("[shellink]"),
		gray("session"),
		cyan(sessionID),
		gray("\u2502"),
		mode,
		gray("\u2502"),
		cyan("Esc") + " " + gray("return"),
		cyan("m") + " " + gray("take control"),
	}, " ")
	fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;1H\x1b[2K%s\x1b[u", rows, padRightAnsi(status, max(0, cols-1)))
}

func setSessionCursor(manual bool, state string) {
	if manual || state == "WAITING_INPUT" {
		fmt.Fprint(os.Stdout, "\x1b[?25h")
		return
	}
	fmt.Fprint(os.Stdout, "\x1b[?25l")
}

func isSessionReturnKey(data []byte) bool {
	return bytes.Equal(data, []byte{0x1b})
}

func dashboardEscapeAction(view string) (nextView string, exit bool) {
	if view == "profiles" {
		return "sessions", false
	}
	return "", true
}

func drawDashboard(size terminalDimensions, view string, items []map[string]any, selected int, query string, searching bool, message string, help bool) {
	cols, rows := size.cols, size.rows
	if cols < 20 {
		cols = 80
	}
	if rows < 8 {
		rows = 24
	}
	var lines []string
	active := 0
	for _, item := range items {
		if item["active"] == true {
			active++
		}
	}
	// Every field a real day needs, and each one in the color that says what it
	// is: green when it is alive, purple when a human holds it, gray when it is
	// gone. Nothing here changes the layout, only what the eye catches first.
	title := strongText("Shellink") + dim("  \u2502  ") + tabLabel(view, "sessions", "1 Sessions") + dim(" \u2502 ") + tabLabel(view, "profiles", "2 Profiles")
	if view == "profiles" {
		title = strongText("Shellink") + dim("  \u2502  ") + tabLabel(view, "sessions", "1 Sessions") + dim(" \u2502 ") + tabLabel(view, "profiles", "2 Profiles")
	}
	lines = append(lines, title)
	if view == "sessions" {
		count, suffix := green(fmt.Sprint(active)), gray(" active, ")+strongText(fmt.Sprint(len(items)))+gray(" total")
		lines = append(lines, fmt.Sprintf("Sessions: %s%s", count, suffix))
	} else {
		lines = append(lines, fmt.Sprintf("Connection profiles: %s", strongText(fmt.Sprint(len(items)))))
	}
	if searching {
		lines = append(lines, dim("Search ")+cyan("/"+query))
	} else if query != "" {
		lines = append(lines, gray("Filter: ")+cyan(query))
	} else {
		lines = append(lines, "")
	}
	if message != "" {
		lines = append(lines, warnSymbol()+" "+yellow(message))
	}
	if help {
		escapeHint := "Esc quit"
		if view == "profiles" {
			escapeHint = "Esc back"
		}
		lines = append(lines, dim(fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			keyHint("j/k", "or arrows move"), keyHint("Enter", "open"), keyHint("1/2", "switch"), escapeHint, keyHint("/", "search"), keyHint("r", "refresh"))))
	} else if len(items) == 0 {
		if view == "sessions" {
			lines = append(lines, gray("No sessions. ")+cyan("2")+gray(" selects a connection profile."))
		} else {
			lines = append(lines, gray("No connection profiles. Create one with ")+cyan("shellink profile create")+gray("."))
		}
	} else {
		start := max(0, selected-rows+8)
		end := min(len(items), start+rows-len(lines)-3)
		if end < selected+1 {
			start = max(0, selected-(rows-len(lines)-4))
			end = min(len(items), start+rows-len(lines)-3)
		}
		for i := start; i < end; i++ {
			item := items[i]
			marker := "  "
			if i == selected {
				marker = cyan(symbolPointer + " ")
			}
			var row string
			if view == "sessions" {
				// Pad first, color after: an escape sequence counts as length in
				// a "%-10s" and would push the next column out of line.
				state, mode := gray(padRightVisible("DISCONNECTED", 14)), padRightVisible("", 7)
				if item["active"] == true {
					state = stateText(padRightVisible(displayValue(item["state"]), 14))
					mode = modeText(padRightVisible(displayValue(item["mode"]), 7))
				}
				id := cyan(padRightVisible(displayValue(item["id"]), 10))
				name := strongText(padRightVisible(displayValue(item["profileName"]), 18))
				row = fmt.Sprintf("%s%s %s %s %s %s", marker, id, state, mode, name, dim(displayValue(item["target"])))
			} else {
				name := strongText(padRightVisible(displayValue(item["name"]), 24))
				kind := blue(padRightVisible(strings.ToUpper(displayValue(item["connectType"])), 8))
				row = fmt.Sprintf("%s%s %s %s", marker, name, kind, displayValue(item["host"]))
			}
			lines = append(lines, row)
		}
	}
	footer := dim(fmt.Sprintf("%s  %s  %s", keyHint("Enter", "open/create"), "Esc quit", keyHint("?", "help")))
	if view == "profiles" {
		footer = dim(fmt.Sprintf("%s  %s  %s", keyHint("Enter", "open/create"), "Esc back", keyHint("?", "help")))
	}
	lines = append(lines, footer)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	fmt.Fprint(os.Stdout, "\x1b[H\x1b[2J")
	for _, line := range lines {
		fmt.Fprint(os.Stdout, line, "\r\n")
	}
}

// tabLabel draws one end of the top bar, brightening whichever view is open.
func tabLabel(view, name, label string) string {
	if view == name {
		return strongText(label)
	}
	return gray(label)
}

// keyHint puts the key in front of what it does, the way every TUI cheat sheet
// does, so the footer can be scanned rather than read.
func keyHint(key, action string) string {
	return cyan(key) + " " + action
}

func displayValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func searchableRow(view string, row map[string]any) string {
	if view == "sessions" {
		return strings.Join([]string{displayValue(row["id"]), displayValue(row["state"]), displayValue(row["mode"]), displayValue(row["profileName"]), displayValue(row["target"])}, " ")
	}
	return strings.Join([]string{displayValue(row["name"]), displayValue(row["connectType"]), displayValue(row["host"]), displayValue(row["username"]), displayValue(row["command"])}, " ")
}

// 宽度感知的 padRightVisible 在 style.go 里（按终端列宽补齐，中文按两列算）。

func readTerminalInput(output chan<- []byte) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			output <- data
		}
		if err != nil {
			close(output)
			return
		}
	}
}
