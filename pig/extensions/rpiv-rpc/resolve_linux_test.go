//go:build linux

package rpc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// fakeProc builds a procfs tree: each process has an exe link and a stat
// line naming its parent.
type fakeProcess struct {
	pid    int
	ppid   int
	exe    string
	name   string
	noStat bool
}

func fakeProc(t *testing.T, processes ...fakeProcess) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range processes {
		dir := filepath.Join(root, strconv.Itoa(p.pid))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if p.exe != "" {
			if err := os.Symlink(p.exe, filepath.Join(dir, "exe")); err != nil {
				t.Fatal(err)
			}
		}
		if !p.noStat {
			stat := strconv.Itoa(p.pid) + " (" + p.name + ") S " + strconv.Itoa(p.ppid) + " 1 1 0 -1 4194560 0\n"
			if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestWalkAncestorsFindsPig(t *testing.T) {
	root := fakeProc(t,
		fakeProcess{pid: 300, ppid: 200, exe: "/tmp/go-build/rpc.test", name: "rpc.test"},
		fakeProcess{pid: 200, ppid: 100, exe: "/usr/bin/bash", name: "bash"},
		fakeProcess{pid: 100, ppid: 50, exe: "/opt/pig/bin/pig", name: "pig"},
		fakeProcess{pid: 50, ppid: 1, exe: "/sbin/init", name: "init"},
	)
	got, err := walkAncestors(root, 300)
	if err != nil || got != "/opt/pig/bin/pig" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestWalkAncestorsStartsAtTheGivenPid(t *testing.T) {
	root := fakeProc(t, fakeProcess{pid: 7, ppid: 1, exe: "/x/pig", name: "pig"})
	if got, err := walkAncestors(root, 7); err != nil || got != "/x/pig" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestWalkAncestorsHandlesOddCommandNames(t *testing.T) {
	root := fakeProc(t,
		fakeProcess{pid: 300, ppid: 200, exe: "/bin/sh", name: "my (weird) name) S 999"},
		fakeProcess{pid: 200, ppid: 1, exe: "/usr/bin/pig (deleted)", name: "pig"},
	)
	got, err := walkAncestors(root, 300)
	if err != nil || got != "/usr/bin/pig" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestWalkAncestorsWithoutPig(t *testing.T) {
	root := fakeProc(t,
		fakeProcess{pid: 300, ppid: 200, exe: "/bin/sh", name: "sh"},
		fakeProcess{pid: 200, ppid: 1, exe: "/sbin/init", name: "init"},
	)
	if _, err := walkAncestors(root, 300); !errors.Is(err, errNoPigAncestor) {
		t.Fatalf("err %v", err)
	}
	// An unreadable parent entry stops the walk with the same error.
	root = fakeProc(t, fakeProcess{pid: 300, ppid: 200, exe: "/bin/sh", name: "sh"}, fakeProcess{pid: 200, exe: "/bin/sh", noStat: true})
	if _, err := walkAncestors(root, 300); !errors.Is(err, errNoPigAncestor) {
		t.Fatalf("err %v", err)
	}
	// A cycle or a very deep chain ends by the step bound.
	root = fakeProc(t, fakeProcess{pid: 300, ppid: 300, exe: "/bin/sh", name: "sh"})
	if _, err := walkAncestors(root, 300); !errors.Is(err, errNoPigAncestor) {
		t.Fatalf("err %v", err)
	}
}

func TestPigFromAncestorsRunsOnThisHost(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no procfs")
	}
	got, err := pigFromAncestors()
	if err == nil && !isPigExecutable(got) {
		t.Fatalf("resolved %q, which is not pig", got)
	}
	if err != nil && !errors.Is(err, errNoPigAncestor) {
		t.Fatalf("err %v", err)
	}
}
