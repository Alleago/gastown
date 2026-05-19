// Package testutil — Dolt process cleanup helpers for tests that spawn
// real `dolt sql-server` processes via `gt install` (or similar) and need
// to reap them reliably at the end of the test.
//
// Why this exists (aa-7f9r): `gt install` forks `dolt sql-server` as a
// long-lived background process — it survives the `gt install` exit by
// design (the server is meant to stick around so subsequent `bd`/`gt`
// commands can talk to it). When the test that spawned it doesn't reach
// its cleanup path (panic, t.Fatal, build cancel, signal, etc.), the
// dolt process gets reparented to init/systemd and lingers indefinitely.
// On one developer host this leaked 77+ orphaned dolt sql-server processes
// over ~11 days, each holding open .dolt LOCK fds on /tmp dirs that the
// test framework had already removed.
//
// ReapDoltOnCleanup() registers a t.Cleanup hook that:
//
//  1. Reads the PID from <townRoot>/daemon/dolt.pid (written by Start()).
//  2. Falls back to <townRoot>/.dolt-data/.dolt/sql-server.info — Dolt's
//     own runtime metadata file — which exists even before the gt
//     daemon/dolt.pid is written (covers crash-during-startup cases).
//  3. Sends SIGTERM to that specific PID, waits up to 2s for it to exit,
//     then escalates to SIGKILL. Process-name verification before kill
//     keeps us from murdering some unrelated PID that happens to be
//     reused.
//  4. As a belt-and-braces fallback, also scans for any `dolt sql-server`
//     whose --config path lives inside the test's townRoot, and reaps
//     those too. This catches the case where the PID file was never
//     written (e.g., Start() died mid-startup).
//
// The cleanup is registered immediately when this helper is called, so it
// fires even if a later assertion in the same test fatals — that is the
// key property the previous `defer gt dolt stop` / `pkill -f` patterns
// lacked.

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ReapDoltOnCleanup registers a t.Cleanup that reaps any dolt sql-server
// process associated with townRoot. Safe to call before `gt install`
// actually runs — the cleanup is a best-effort no-op when there's nothing
// to reap.
//
// Call this immediately after constructing townRoot, BEFORE invoking
// `gt install` (or anything else that may spawn dolt). That way, even if
// the install command itself panics, the cleanup still fires.
func ReapDoltOnCleanup(t *testing.T, townRoot string) {
	t.Helper()
	t.Cleanup(func() { reapDolt(t, townRoot) })
}

// reapDolt is the cleanup body. Exposed as a free function so it can be
// tested directly and called from non-t.Cleanup contexts if needed.
func reapDolt(t *testing.T, townRoot string) {
	t.Helper()

	pids := collectDoltPIDs(townRoot)
	if len(pids) == 0 {
		return
	}

	// First pass: SIGTERM (graceful).
	for pid := range pids {
		if !looksLikeDoltSQLServer(pid) {
			delete(pids, pid)
			continue
		}
		if err := signalProcess(pid, "TERM"); err != nil {
			// Best-effort; the process may have already exited between
			// our discovery and the signal.
			t.Logf("dolt reap: SIGTERM to PID %d failed: %v", pid, err)
		}
	}

	// Wait for graceful shutdown, polling.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		anyAlive := false
		for pid := range pids {
			if processIsAlive(pid) {
				anyAlive = true
				break
			}
		}
		if !anyAlive {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Escalate: SIGKILL the stragglers.
	for pid := range pids {
		if !processIsAlive(pid) {
			continue
		}
		if err := signalProcess(pid, "KILL"); err != nil {
			t.Logf("dolt reap: SIGKILL to PID %d failed: %v", pid, err)
		}
	}
}

// collectDoltPIDs returns the set of candidate dolt sql-server PIDs
// associated with townRoot. Deduplicated across the three discovery
// strategies (PID file, sql-server.info, /proc scan).
func collectDoltPIDs(townRoot string) map[int]struct{} {
	pids := make(map[int]struct{})

	// Strategy 1: <townRoot>/daemon/dolt.pid (written by doltserver.Start).
	if pid, ok := readPIDFile(filepath.Join(townRoot, "daemon", "dolt.pid")); ok {
		pids[pid] = struct{}{}
	}

	// Strategy 2: <townRoot>/.dolt-data/.dolt/sql-server.info (Dolt's own
	// metadata; format is "PID:PORT[:SECRET]"). Present even if gt's PID
	// file was never written.
	if pid, ok := readDoltSQLServerInfoPID(filepath.Join(townRoot, ".dolt-data", ".dolt", "sql-server.info")); ok {
		pids[pid] = struct{}{}
	}

	// Strategy 3 (Linux only): scan /proc for `dolt sql-server` processes
	// whose --config path lives inside townRoot. This is the catch-all
	// for processes that escaped both PID file mechanisms (e.g., the
	// pidfile write failed, or Dolt itself wrote a different
	// sql-server.info location).
	for _, pid := range scanProcForDoltInTown(townRoot) {
		pids[pid] = struct{}{}
	}

	return pids
}

// readPIDFile reads a file containing a single integer PID. Returns
// (0, false) if the file is missing, empty, or unparseable.
func readPIDFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 1 {
		return 0, false
	}
	return pid, true
}

// readDoltSQLServerInfoPID parses Dolt's sql-server.info file
// (format: "PID:PORT" or "PID:PORT:SECRET") and returns the PID.
func readDoltSQLServerInfoPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 3)
	if len(parts) < 2 {
		return 0, false
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 1 {
		return 0, false
	}
	return pid, true
}

// signalProcess sends a named signal to a PID using `kill`. Going through
// the `kill` binary (rather than syscall.Kill) keeps this file portable
// across GOOS values without a separate _windows variant — Windows
// integration tests skip on dolt-server scenarios anyway, and the helper
// is a no-op there because no PIDs would have been discovered.
func signalProcess(pid int, sig string) error {
	cmd := exec.Command("kill", "-"+sig, strconv.Itoa(pid))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kill -%s %d: %v (%s)", sig, pid, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// processIsAlive reports whether a PID currently refers to a running
// process. Uses `kill -0` (no signal sent, just permission check) which
// is portable across POSIX systems.
func processIsAlive(pid int) bool {
	// FindProcess always succeeds on Unix, so we use the kill -0 idiom
	// instead. Errors mean either ESRCH (no such pid) or EPERM (process
	// exists but we can't signal it). Treat EPERM as "alive" — better to
	// over-report than skip a kill.
	cmd := exec.Command("kill", "-0", strconv.Itoa(pid))
	return cmd.Run() == nil
}
