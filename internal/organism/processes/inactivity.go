package processes

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
)

// InactivityReport contains observed facts only. It does not infer an exit from
// log wording, and never recommends a command or signals the process.
func InactivityReport(identifier string, activity process.Activity, stdout, stderr []byte) string {
	return fmt.Sprintf("process %s: state=running; no output for %d ms; last activity=%s; observed=%s\nstdout (last 20 lines / 4096 bytes):\n%s\nstderr (last 20 lines / 4096 bytes):\n%s", identifier, activity.ObservedAt.Sub(activity.LastOutputAt).Milliseconds(), activity.LastOutputAt.UTC().Format("2006-01-02T15:04:05.000Z"), activity.ObservedAt.UTC().Format("2006-01-02T15:04:05.000Z"), processOutputTail(stdout), processOutputTail(stderr))
}

func processOutputTail(data []byte) string {
	if len(data) == 0 {
		return "(no output)"
	}
	if len(data) > 4096 {
		data = data[len(data)-4096:]
		for len(data) > 0 && !utf8.RuneStart(data[0]) {
			data = data[1:]
		}
	}
	text := string(data)
	end := len(text)
	if strings.HasSuffix(text, "\n") {
		end--
	}
	for count := 0; count < 20; count++ {
		position := strings.LastIndexByte(text[:end], '\n')
		if position < 0 {
			return text
		}
		if count == 19 {
			return text[position+1:]
		}
		end = position
	}
	return text
}
