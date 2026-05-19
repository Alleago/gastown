//go:build !linux

package testutil

import (
	"os/exec"
	"strconv"
	"strings"
)

// scanProcForDoltInTown is a no-op on non-Linux platforms. The PID-file
// and sql-server.info strategies still work; we just lose the catch-all
// /proc scan. macOS tests that reach the leak path are rare in practice
// because most integration tests run in Docker/CI on Linux.
func scanProcForDoltInTown(townRoot string) []int {
	return nil
}

// looksLikeDoltSQLServer verifies a PID via `ps -o command=`. Works on
// macOS and BSD; on Windows it falls back to "true" (Windows tests skip
// the spawn-dolt scenarios via build tags).
func looksLikeDoltSQLServer(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	cmdline := strings.TrimSpace(string(out))
	if cmdline == "" {
		return false
	}
	// Must start with "dolt" (possibly an absolute path) and contain
	// "sql-server" as the next token.
	fields := strings.Fields(cmdline)
	if len(fields) < 2 {
		return false
	}
	exe := fields[0]
	if !(exe == "dolt" || strings.HasSuffix(exe, "/dolt")) {
		return false
	}
	return fields[1] == "sql-server"
}
