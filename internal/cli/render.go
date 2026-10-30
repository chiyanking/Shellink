package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Everything in this file turns one JSON value into text a person can read.
//
// The rules are the ones good command line tools converged on: answers line up
// in columns so they can be compared downwards instead of parsed; states carry
// a color so a problem is visible before it is read; long output is cut to the
// width of the terminal instead of wrapping into noise; and machine consumers
// are left alone, because --json never passes through here.

// Columns are offered in this order when a table needs more than the object
// actually carries. Anything unknown is appended alphabetically, which keeps a
// table stable between runs even when one row carries an extra field.
var tableColumnOrder = []string{
	"id", "name", "profileId", "profileName", "target", "host", "connectType", "type",
	"state", "mode", "active", "running", "status", "path", "size", "sizeBytes",
	"url", "createdAt", "updatedAt", "lastUsedAt", "username",
}

func render(value any) string { return renderIndent(value, "") }

func renderIndent(value any, indent string) string {
	switch typed := value.(type) {
	case nil:
		return gray("-")
	case map[string]any:
		return renderObject(typed, indent)
	case []any:
		return renderArray(typed, indent)
	case string:
		return renderString("", typed, indent)
	case bool:
		return boolText(typed)
	case float64:
		return cyan(strconv.FormatFloat(typed, 'f', -1, 64))
	case float32:
		return cyan(strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case int:
		return cyan(strconv.Itoa(typed))
	case int64:
		return cyan(strconv.FormatInt(typed, 10))
	case json.Number:
		return cyan(typed.String())
	default:
		return fmt.Sprint(typed)
	}
}

// renderObject lines a map up as key/value rows aligned on the widest key, with
// nested structures folded under the key that owns them.
func renderObject(value map[string]any, indent string) string {
	if len(value) == 0 {
		return indent + gray("(empty)")
	}
	keys := orderedKeys(value)
	width := 0
	for _, key := range keys {
		width = max(width, visibleWidth(key))
	}
	var lines []string
	for _, key := range keys {
		field := value[key]
		label := keyText(padRightVisible(key, width))
		switch typed := field.(type) {
		case map[string]any:
			if len(typed) == 0 {
				lines = append(lines, fmt.Sprintf("%s%s  %s", indent, label, gray("(empty)")))
				continue
			}
			// One field under one key is not worth a block of its own:
			// "webui.url  http://..." keeps the eye on a single line.
			if only, nested, ok := singleScalar(typed); ok {
				lines = append(lines, fmt.Sprintf("%s%s.%s  %s", indent, keyText(key), only, renderField(only, nested, indent+"  ")))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s%s:", indent, label))
			lines = append(lines, renderObject(typed, indent+"  "))
		case []any:
			if len(typed) == 0 {
				lines = append(lines, fmt.Sprintf("%s%s  %s", indent, label, gray("(none)")))
				continue
			}
			if rows, ok := rowsOf(typed); ok {
				lines = append(lines, fmt.Sprintf("%s%s:", indent, label))
				lines = append(lines, renderTable(rows, indent+"  "))
				continue
			}
			if list, ok := stringsOnly(typed); ok {
				lines = append(lines, fmt.Sprintf("%s%s  %s", indent, label, strings.Join(list, gray(", "))))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s%s:", indent, label))
			lines = append(lines, renderIndent(typed, indent+"  "))
		default:
			// Text that spans lines gets the whole width under its own key:
			// putting the first line beside the label would indent the rest at
			// a different column and make command output look mangled.
			if text, ok := field.(string); ok && strings.ContainsAny(text, "\r\n") {
				lines = append(lines, fmt.Sprintf("%s%s:", indent, label))
				lines = append(lines, blockLines(text, indent+"  "))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s", indent, label, renderField(key, field, indent+"  ")))
		}
	}
	return strings.Join(lines, "\n")
}

// renderArray prints a list of objects as a table and anything else as a list,
// because a table of strings reads like noise.
func renderArray(value []any, indent string) string {
	if len(value) == 0 {
		return indent + gray("(none)")
	}
	if rows, ok := rowsOf(value); ok {
		return renderTable(rows, indent)
	}
	if list, ok := stringsOnly(value); ok {
		lines := make([]string, 0, len(list))
		for _, item := range list {
			lines = append(lines, fmt.Sprintf("%s%s %s", indent, gray(symbolPointer), renderString("", item, indent)))
		}
		return strings.Join(lines, "\n")
	}
	lines := make([]string, 0, len(value))
	for _, item := range value {
		lines = append(lines, fmt.Sprintf("%s%s %s", indent, gray("-"), renderIndent(item, indent+"  ")))
	}
	return strings.Join(lines, "\n")
}

// renderTable formats rows into aligned columns under a dim header. Widths come
// from the content, never from a guess, and are squeezed to the terminal width
// so a long target never pushes the state column off screen.
func renderTable(rows []map[string]any, indent string) string {
	columns := tableColumns(rows)
	if len(columns) == 0 {
		return indent + gray("(none)")
	}
	// One row also carries the gaps between columns; keep them in the budget so
	// nothing wraps in a narrow window.
	budget := max(40, outputWidth()-visibleWidth(indent)-(len(columns)-1)*2)
	widths := columnWidths(rows, columns)
	// Objects here carry far more fields than a terminal is wide. Squeezing
	// every column down to eight characters turns the header into noise, so the
	// overflowing tail is dropped instead and the count is printed underneath:
	// nothing is hidden silently, and --json still carries every field.
	dropped := 0
	for sum(widths) > budget && len(columns) > 3 {
		columns = columns[:len(columns)-1]
		widths = columnWidths(rows, columns)
		dropped++
	}
	for sum(widths) > budget {
		widest, best := 0, -1
		for column, width := range widths {
			if width > widest {
				widest, best = width, column
			}
		}
		if best < 0 || widest <= 8 {
			break
		}
		shrink := min(widest-8, sum(widths)-budget)
		widths[best] -= shrink
	}

	var lines []string
	header := make([]string, len(columns))
	rule := make([]string, len(columns))
	for column, key := range columns {
		header[column] = fitVisible(headText(key), widths[column])
		rule[column] = strings.Repeat("\u2500", widths[column])
	}
	lines = append(lines, indent+strings.TrimRight(strings.Join(header, gray("  ")), " "))
	lines = append(lines, indent+gray(strings.Join(rule, gray("\u2500\u2500"))))
	for _, item := range rows {
		cells := make([]string, len(columns))
		for column, key := range columns {
			cells[column] = fitVisible(fieldText(key, item[key]), widths[column])
		}
		lines = append(lines, indent+strings.TrimRight(strings.Join(cells, "  "), " "))
	}
	if dropped > 0 {
		lines = append(lines, indent+gray(fmt.Sprintf("\u2026 %d more %s · use --json for everything", dropped, plural(dropped, "column"))))
	}
	return strings.Join(lines, "\n")
}

// columnWidths measures every column as wide as its widest cell or header,
// whichever wins.
func columnWidths(rows []map[string]any, columns []string) []int {
	widths := make([]int, len(columns))
	for _, item := range rows {
		for column, key := range columns {
			widths[column] = max(widths[column], visibleWidth(plainCell(key, item[key])))
		}
	}
	for column, key := range columns {
		widths[column] = max(widths[column], visibleWidth(key))
	}
	return widths
}

func plural(count int, word string) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

// fieldText renders one cell: colored by meaning, never wider than needed.
func fieldText(key string, value any) string {
	if value == nil {
		return gray("-")
	}
	switch typed := value.(type) {
	case bool:
		return boolText(typed)
	case string:
		return cellString(key, typed)
	case float64:
		if key == "sizeBytes" || key == "size" {
			return yellow(humanSize(int64(typed)))
		}
		return cyan(strconv.FormatFloat(typed, 'f', -1, 64))
	case map[string]any, []any:
		return dim(structuredSummary(typed))
	default:
		text := ansiAware(renderIndent(value, ""))
		return firstLine(text)
	}
}

// plainCell is the same cell without color, used to measure widths: alignment
// has to follow what the eye sees, not what the bytes are.
func plainCell(key string, value any) string { return ansiAware(fieldText(key, value)) }

// blockLines prints several lines under a grey marker so the block reads as one
// answer, with nothing reformatted and the trailing blank lines dropped.
func blockLines(value string, indent string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = indent + gray("\u2502 ") + lines[index]
	}
	for len(lines) > 0 && strings.TrimSpace(ansiAware(lines[len(lines)-1])) == "\u2502" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// renderField renders the value of one key/value row.
func renderField(key string, value any, indent string) string {
	text, ok := value.(string)
	if !ok {
		return renderIndent(value, indent)
	}
	if !strings.ContainsAny(text, "\r\n") {
		return cellString(key, text)
	}
	return blockLines(text, indent)
}

// renderField renders the value of one key/value row.

// renderString renders a bare string answer, keeping whatever newlines it came
// with.
func renderString(key, value, indent string) string {
	if value == "" {
		return gray("(empty)")
	}
	if !strings.ContainsAny(value, "\r\n") {
		return cellString(key, value)
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = indent + gray("\u2502 ") + lines[index]
	}
	return strings.Join(lines, "\n")
}

// cellString is one string cell, aware that some fields are states rather than
// words.
func cellString(key, value string) string {
	text := strings.TrimRight(value, "\r\n")
	if strings.ContainsAny(text, "\r\n") {
		return firstLine(text)
	}
	switch strings.ToLower(key) {
	case "state":
		return stateText(text)
	case "mode":
		return modeText(text)
	case "id":
		return cyan(text)
	case "status":
		return statusText(text)
	case "url", "webui", "webuiurl":
		return paint(ansiBlue, text)
	}
	if looksLikeURL(text) {
		return paint(ansiBlue, text)
	}
	if isTimestampKey(key) {
		return dim(text)
	}
	switch strings.ToLower(key) {
	case "active", "running":
		if strings.EqualFold(text, "true") || strings.EqualFold(text, "false") {
			return boolText(strings.EqualFold(text, "true"))
		}
	}
	return text
}

// singleScalar picks a lone plain field out of a nested object, or reports that
// the object has more going on than that.
func singleScalar(value map[string]any) (string, any, bool) {
	if len(value) != 1 {
		return "", nil, false
	}
	for key, nested := range value {
		if isContainer(nested) {
			return "", nil, false
		}
		return key, nested, true
	}
	return "", nil, false
}

func isContainer(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func rowsOf(value []any) ([]map[string]any, bool) {
	if len(value) == 0 {
		return nil, false
	}
	rows := make([]map[string]any, 0, len(value))
	for _, item := range value {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func stringsOnly(value []any) ([]string, bool) {
	list := make([]string, 0, len(value))
	for _, item := range value {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		list = append(list, text)
	}
	return list, true
}

// tableColumns keeps the well-known fields in front and appends the rest in
// alphabetical order, so the header is predictable but loses nothing.
func tableColumns(rows []map[string]any) []string {
	seen := map[string]bool{}
	var columns []string
	for _, key := range tableColumnOrder {
		for _, row := range rows {
			if _, ok := row[key]; ok {
				seen[key] = true
				columns = append(columns, key)
				break
			}
		}
	}
	var extra []string
	for _, row := range rows {
		for key := range row {
			if !seen[key] {
				seen[key] = true
				extra = append(extra, key)
			}
		}
	}
	sort.Strings(extra)
	return append(columns, extra...)
}

func orderedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left, right := columnRank(keys[i]), columnRank(keys[j])
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}

func columnRank(key string) int {
	for index, candidate := range tableColumnOrder {
		if candidate == key {
			return index
		}
	}
	return len(tableColumnOrder)
}

func structuredSummary(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		return fmt.Sprintf("%d fields", len(typed))
	case []any:
		return fmt.Sprintf("%d items", len(typed))
	default:
		return fmt.Sprint(value)
	}
}

func firstLine(text string) string {
	head, _, _ := strings.Cut(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	return strings.TrimSpace(head)
}

func looksLikeURL(text string) bool {
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")
}

func isTimestampKey(key string) bool {
	switch strings.ToLower(key) {
	case "createdat", "updatedat", "lastusedat", "timestamp", "startedat", "finishedat", "endedat", "time":
		return true
	}
	return false
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// outputWidth is how much room an answer gets. A pipe has no width, so those
// keep a comfortable default instead of being cut into garbage.
func outputWidth() int {
	if text := os.Getenv("COLUMNS"); text != "" {
		if value, err := strconv.Atoi(text); err == nil && value >= 40 {
			return min(value, 250)
		}
	}
	if isTTY(os.Stdout) {
		if dimensions := getTerminalDimensions(); dimensions.cols >= 40 {
			return min(dimensions.cols, 250)
		}
	}
	return 120
}

// ansiAware drops escape sequences, which is how colored cells are measured.
func ansiAware(value string) string {
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == 0x1b {
			for i < len(value) && value[i] != 'm' {
				i++
			}
			continue
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
