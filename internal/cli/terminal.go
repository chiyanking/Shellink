package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"shellink/internal/client"
	"shellink/internal/config"
	"shellink/internal/protocol"
)

type terminalDimensions struct{ cols, rows int }

func runInteractive(cfg config.Config) error {
	connection, err := client.Connect(cfg, 10*time.Second)
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
			case bytes.Contains(data, []byte{0x03}) || bytes.Contains(data, []byte("q")):
				return nil
			case bytes.Equal(data, []byte("1")), bytes.Equal(data, []byte("\t")), bytes.Contains(data, []byte("\x1b[D")):
				view, selected, query, showHelp = "sessions", 0, "", false
				refresh()
			case bytes.Equal(data, []byte("2")), bytes.Contains(data, []byte("\x1b[C")):
				view, selected, query, showHelp = "profiles", 0, "", false
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
						if err := runSessionTerminal(connection, input, ticker.C, id, dimensions); err != nil {
							message = err.Error()
						}
					} else {
						id, _ := items[selected]["id"].(string)
						if err := runSessionTerminal(connection, input, ticker.C, id, dimensions); err != nil {
							message = err.Error()
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
	mode, _ := protocol.StringValue(state["mode"])
	manual := mode == "MANUAL"
	tookControl := false
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
	if replay, ok := protocol.StringValue(initial["replay"]); ok {
		fmt.Fprint(os.Stdout, replay)
	}
	fmt.Fprintf(os.Stdout, "\r\n[shellink] session %s | %s | Ctrl+] return | m take control\r\n", sessionID, modeLabel(manual))
	_ = resizeSession(connection, sessionID, dimensions)
	for {
		select {
		case <-resizePoll:
			if current := getTerminalDimensions(); current != dimensions {
				dimensions = current
				_ = resizeSession(connection, sessionID, dimensions)
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
				fmt.Fprintf(os.Stdout, "\r\n[shellink] mode: %s\r\n", modeLabel(manual))
			case "session.closed":
				reason, _ := protocol.StringValue(data["reason"])
				fmt.Fprintf(os.Stdout, "\r\n[shellink] session closed: %s\r\n", reason)
			}
		case data, ok := <-input:
			if !ok || data == nil {
				return nil
			}
			if bytes.Equal(data, []byte{0x1d}) || bytes.Contains(data, []byte("\x1b[93;5u")) {
				if tookControl {
					_, _ = connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "AUTO"}), 5*time.Second)
				}
				unsubscribeEvents(connection, subscriptionID)
				return nil
			}
			if !manual && bytes.Equal(data, []byte("m")) {
				if _, err := connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "MANUAL"}), 5*time.Second); err != nil {
					fmt.Fprintf(os.Stdout, "\r\n[shellink] %v\r\n", err)
				} else {
					manual, tookControl = true, true
					fmt.Fprint(os.Stdout, "\r\n[shellink] manual control\r\n")
				}
				continue
			}
			if manual && (bytes.Equal(data, []byte{0x1c}) || bytes.Contains(data, []byte("\x1b[92;5u"))) {
				if _, err := connection.Request("sessions.mode", protocol.JSONValue(map[string]any{"id": sessionID, "mode": "AUTO"}), 5*time.Second); err == nil {
					manual, tookControl = false, false
					fmt.Fprint(os.Stdout, "\r\n[shellink] automatic control\r\n")
				}
				continue
			}
			if manual {
				_, err := connection.Request("sessions.input", protocol.JSONValue(map[string]any{"id": sessionID, "text": string(data), "appendNewline": false, "manual": true}), 5*time.Second)
				if err != nil {
					fmt.Fprintf(os.Stdout, "\r\n[shellink] %v\r\n", err)
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

func modeLabel(manual bool) string {
	if manual {
		return "MANUAL"
	}
	return "AUTO (read only)"
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
	title := "Shellink | 1 Sessions | 2 Profiles"
	if view == "profiles" {
		title = "Shellink | 1 Sessions | 2 Profiles"
	}
	lines = append(lines, padRightVisible(title, cols))
	if view == "sessions" {
		lines = append(lines, padRightVisible(fmt.Sprintf("Sessions: %d active, %d total", active, len(items)), cols))
	} else {
		lines = append(lines, padRightVisible(fmt.Sprintf("Connection profiles: %d", len(items)), cols))
	}
	if searching {
		lines = append(lines, padRightVisible("Search /"+query, cols))
	} else if query != "" {
		lines = append(lines, padRightVisible("Filter: "+query, cols))
	} else {
		lines = append(lines, "")
	}
	if message != "" {
		lines = append(lines, padRightVisible(message, cols))
	}
	if help {
		lines = append(lines, "j/k or arrows move   Enter open   1/2 switch   / search   r refresh   q quit")
	} else if len(items) == 0 {
		if view == "sessions" {
			lines = append(lines, "No sessions. Press 2 to select a connection profile.")
		} else {
			lines = append(lines, "No connection profiles. Create one with `shellink profile create`.")
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
				marker = "> "
			}
			var row string
			if view == "sessions" {
				status, mode := "", ""
				if item["active"] == true {
					status, _ = item["state"].(string)
					mode, _ = item["mode"].(string)
				}
				row = fmt.Sprintf("%s%-10s %-14s %-7s %-18s %s", marker, displayValue(item["id"]), status, mode, displayValue(item["profileName"]), displayValue(item["target"]))
			} else {
				row = fmt.Sprintf("%s%-24s %-8s %s", marker, displayValue(item["name"]), strings.ToUpper(displayValue(item["connectType"])), displayValue(item["host"]))
			}
			lines = append(lines, padRightVisible(row, cols))
		}
	}
	lines = append(lines, padRightVisible("Enter open/create   m take control   Ctrl+] return   ? help   q quit", cols))
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

func padRightVisible(value string, width int) string {
	runes := []rune(value)
	if len(runes) > width {
		runes = runes[:max(0, width)]
	}
	return string(runes) + strings.Repeat(" ", max(0, width-len(runes)))
}

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
