//go:build linux

package rpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxAncestors bounds the parent walk; a process tree deeper than this is not
// one pig started.
const maxAncestors = 64

// pigFromAncestors walks this process's parents through procfs and returns
// the executable of the first one named pig.
func pigFromAncestors() (string, error) {
	return walkAncestors("/proc", os.Getppid())
}

// walkAncestors starts at pid and follows the parent links in
// <procRoot>/<pid>/stat, reading each process's exe link. It stops at init, at
// an unreadable entry or after maxAncestors steps.
func walkAncestors(procRoot string, pid int) (string, error) {
	for range maxAncestors {
		if pid <= 1 {
			break
		}
		dir := filepath.Join(procRoot, strconv.Itoa(pid))
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil && isPigExecutable(exe) {
			return strings.TrimSuffix(exe, " (deleted)"), nil
		}
		parent, err := parentPID(filepath.Join(dir, "stat"))
		if err != nil {
			return "", fmt.Errorf("%w (stopped at pid %d: %v)", errNoPigAncestor, pid, err)
		}
		pid = parent
	}
	return "", errNoPigAncestor
}

// parentPID reads the ppid field of a /proc/<pid>/stat file. The command name
// in parentheses may contain spaces and parentheses, so the fields are taken
// after the last closing parenthesis: state, then ppid.
func parentPID(statPath string) (int, error) {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}
	stat := string(data)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, fmt.Errorf("malformed stat %q", statPath)
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed stat %q", statPath)
	}
	return strconv.Atoi(fields[1])
}
