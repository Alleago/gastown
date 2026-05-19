//go:build linux

package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// scanProcForDoltInTown walks /proc looking for `dolt sql-server`
// processes whose --config argument lives inside townRoot. Linux-only
// because /proc/<pid>/cmdline is the simplest portable way to inspect
// process arguments without spawning `ps`.
//
// Returns an empty slice on any error or on platforms without /proc.
func scanProcForDoltInTown(townRoot string) []int {
	absRoot, err := filepath.Abs(townRoot)
	if err != nil {
		return nil
	}
	// Trailing separator guards against substring false-positives
	// (e.g. /tmp/townA vs /tmp/townA-other).
	rootWithSep := absRoot + string(filepath.Separator)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// /proc/<pid>/cmdline uses NUL separators between argv entries.
		args := bytes.Split(cmdline, []byte{0})
		if len(args) < 2 {
			continue
		}
		// argv[0] is "dolt" (or absolute path ending in /dolt) and
		// argv[1] is "sql-server". Anything else is not our target.
		exe := string(args[0])
		if !(exe == "dolt" || strings.HasSuffix(exe, "/dolt")) {
			continue
		}
		if string(args[1]) != "sql-server" {
			continue
		}
		// Look for an argument that points into townRoot. Both
		// "--config /path" (two args) and "--config=/path" (one arg)
		// are accepted; same for --data-dir.
		match := false
		for i, raw := range args {
			a := string(raw)
			if a == "" {
				continue
			}
			// Standalone "--config /path" or "--data-dir /path"
			if (a == "--config" || a == "--data-dir") && i+1 < len(args) {
				next := string(args[i+1])
				if next == absRoot || strings.HasPrefix(next, rootWithSep) {
					match = true
					break
				}
			}
			// Inlined "--config=/path" or "--data-dir=/path"
			for _, prefix := range []string{"--config=", "--data-dir="} {
				if strings.HasPrefix(a, prefix) {
					val := a[len(prefix):]
					if val == absRoot || strings.HasPrefix(val, rootWithSep) {
						match = true
					}
				}
			}
			if match {
				break
			}
		}
		if match {
			pids = append(pids, pid)
		}
	}
	return pids
}

// looksLikeDoltSQLServer verifies that pid is actually a `dolt sql-server`
// process. This guards against killing some unrelated PID that has been
// reused since the PID file was written.
func looksLikeDoltSQLServer(pid int) bool {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		// If we can't read /proc/<pid>/cmdline the process is probably
		// gone (or we lack permission). Either way it's safe to skip.
		return false
	}
	args := bytes.Split(cmdline, []byte{0})
	if len(args) < 2 {
		return false
	}
	exe := string(args[0])
	if !(exe == "dolt" || strings.HasSuffix(exe, "/dolt")) {
		return false
	}
	return string(args[1]) == "sql-server"
}
