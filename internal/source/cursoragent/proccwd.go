package cursoragent

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// processCwd shells out to `lsof -a -p <pid> -d cwd -Fn` to find pid's
// current working directory. gopsutil's own CwdWithContext is
// unimplemented on darwin without cgo (this project's release build
// target, CGO_ENABLED=0) — this mirrors gopsutil's OWN fallback strategy
// for the sibling ExeWithContext call on that exact platform/build
// combination (see process_darwin_nocgo.go in the gopsutil source),
// rather than introducing a new, unprecedented pattern. Confirmed output
// shape on this machine: three lines, "p<pid>", "fcwd", "n<path>" — only
// the "n"-prefixed line is read. Fails closed (ok=false) if lsof isn't on
// PATH, the process is gone, or output doesn't parse — never guesses.
func processCwd(ctx context.Context, pid int) (string, bool) {
	out, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:], true
		}
	}
	return "", false
}
