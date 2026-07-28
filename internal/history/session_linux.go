package history

import (
	"fmt"
	"os"
	"strings"
)

// shellStartToken returns the process start time in clock ticks since boot,
// read from /proc. An empty string means it could not be determined.
func shellStartToken(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}

	// Field 2 is the executable name in parentheses and may itself contain
	// spaces and parentheses, so parsing starts after the final ')'. The
	// fields that follow begin at field 3 (state), making starttime -- field
	// 22 -- index 19.
	stat := string(data)
	close := strings.LastIndex(stat, ")")
	if close < 0 {
		return ""
	}
	fields := strings.Fields(stat[close+1:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
