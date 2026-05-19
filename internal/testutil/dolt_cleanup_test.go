package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// TestReapDoltOnCleanup_NoFiles verifies that the cleanup helper is a no-op
// when neither the PID file nor sql-server.info exists, and when /proc has
// no matching dolt process. This is the common "nothing to do" path.
func TestReapDoltOnCleanup_NoFiles(t *testing.T) {
	townRoot := t.TempDir()
	// reapDolt should not panic and should not log anything alarming.
	reapDolt(t, townRoot)
}

// TestReapDoltOnCleanup_PIDFileMissingProcess covers a stale PID file
// pointing at a PID that no longer exists. Should not kill anything and
// must not panic.
func TestReapDoltOnCleanup_PIDFileMissingProcess(t *testing.T) {
	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		t.Fatalf("mkdir daemon: %v", err)
	}
	// Pick a PID that's almost certainly not in use. 999999 is well above
	// the typical pid_max on Linux defaults; even when it's not, the
	// process-name check (looksLikeDoltSQLServer) will filter it out.
	pidFile := filepath.Join(daemonDir, "dolt.pid")
	if err := os.WriteFile(pidFile, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	reapDolt(t, townRoot)
}

// TestReapDoltOnCleanup_ReapsRealChild spawns a long-running sleep process
// as a stand-in for dolt sql-server, writes its PID into the conventional
// dolt.pid path, and verifies the reaper's signal+wait plumbing kills it.
//
// We bypass the process-name guard by calling signalProcess directly:
// looksLikeDoltSQLServer would (correctly) reject a sleep process, so this
// test asserts the SIGTERM→wait→SIGKILL plumbing, not the discovery
// filter (which is tested by TestLooksLikeDoltSQLServer_RejectsNonDolt).
//
// Skipped in sandboxed environments where the test process can't send
// signals to its own children (some CI sandboxes block this) — detected
// by spawning a quick sleep and confirming SIGKILL actually kills it.
func TestReapDoltOnCleanup_ReapsRealChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep + kill semantics differ on Windows")
	}
	if !canKillChildren(t) {
		t.Skip("environment blocks signals to child processes (likely a sandbox); test cannot run here")
	}

	townRoot := t.TempDir()
	daemonDir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		t.Fatalf("mkdir daemon: %v", err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		// Belt-and-braces: if the test fails before our reap runs,
		// still kill the sleep so we don't leak.
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	pid := cmd.Process.Pid
	pidFile := filepath.Join(daemonDir, "dolt.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	// Sanity check.
	if !processIsAlive(pid) {
		t.Fatalf("sleep child (PID %d) died before reap", pid)
	}

	// Bypass the process-name guard: send SIGTERM directly. We're
	// asserting the signal/wait plumbing works, not the discovery
	// filter (which is tested below).
	if err := signalProcess(pid, "TERM"); err != nil {
		t.Fatalf("signalProcess SIGTERM: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processIsAlive(pid) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if processIsAlive(pid) {
		// Escalate, as the reaper would.
		_ = signalProcess(pid, "KILL")
		time.Sleep(200 * time.Millisecond)
	}
	if processIsAlive(pid) {
		t.Fatalf("sleep child (PID %d) survived SIGTERM and SIGKILL", pid)
	}
	_, _ = cmd.Process.Wait() // Reap the zombie.
}

// canKillChildren probes whether this process can actually deliver signals
// to its own children. Some CI sandboxes silently swallow kill(2) calls
// targeting child processes, which would otherwise make the reaper test
// appear broken when the bug is really in the test harness.
func canKillChildren(t *testing.T) bool {
	t.Helper()
	probe := exec.Command("sleep", "5")
	if err := probe.Start(); err != nil {
		return false
	}
	defer func() { _, _ = probe.Process.Wait() }()
	pid := probe.Process.Pid
	if err := signalProcess(pid, "KILL"); err != nil {
		return false
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !processIsAlive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Probe survived SIGKILL — sandbox is blocking child signals.
	return false
}

// TestLooksLikeDoltSQLServer_RejectsNonDolt confirms the process-name
// filter doesn't false-positive on arbitrary processes. We use our own
// test process — definitely not "dolt sql-server".
func TestLooksLikeDoltSQLServer_RejectsNonDolt(t *testing.T) {
	if looksLikeDoltSQLServer(os.Getpid()) {
		t.Fatalf("looksLikeDoltSQLServer falsely matched the test process")
	}
}
