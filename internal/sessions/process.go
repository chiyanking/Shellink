package sessions

import (
	"os"
	"strings"
)

type Transport interface {
	Write(string) error
	Resize(cols, rows int) error
	Close() error
	Output() <-chan string
	Done() <-chan error
}

func environment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		if index := strings.IndexByte(entry, '='); index > 0 {
			values[entry[:index]] = entry[index+1:]
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
