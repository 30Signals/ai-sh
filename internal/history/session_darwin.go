package history

import (
	"os/exec"
	"strconv"
	"strings"
)

// shellStartToken returns the process start time as reported by ps. There is no
// /proc on darwin, so this shells out; the cost is negligible next to the
// inference call that follows. An empty string means it could not be determined.
func shellStartToken(pid int) string {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(out)), "")
}
