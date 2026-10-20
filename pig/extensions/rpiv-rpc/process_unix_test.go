//go:build unix

package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// processGone reports whether pid no longer runs: it is absent from procfs
// or a zombie (a killed grandchild stays a zombie when nothing reaps it).
func processGone(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	stat := string(data)
	fields := strings.Fields(stat[strings.LastIndexByte(stat, ')')+1:])
	return len(fields) > 0 && fields[0] == "Z"
}

func TestCloseKillsProcessGroup(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no procfs")
	}
	c := startFake(t, fakeIgnoreEOFEnv+"=1", fakeHolderEnv+"=1")
	col := collect(c)
	col.waitFor(t, "holder")
	col.mu.Lock()
	raw := col.events[0].Raw
	col.mu.Unlock()
	var holder struct {
		Pid int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &holder); err != nil || holder.Pid <= 0 {
		t.Fatalf("holder record %s: %v", raw, err)
	}
	c.Close()
	var exitErr *exec.ExitError
	if err := c.Wait(); !errors.As(err, &exitErr) {
		t.Fatalf("Wait: %v", err)
	}
	if status, ok := exitErr.Sys().(syscall.WaitStatus); !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("child was not killed: %v", exitErr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !processGone(holder.Pid) {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d survived the process group kill", holder.Pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
