package rpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeExecutable writes an executable script named name under dir.
func fakeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolvePigPrecedence(t *testing.T) {
	root := t.TempDir()
	override := fakeExecutable(t, filepath.Join(root, "override"), "my-pig")
	fromEnv := fakeExecutable(t, filepath.Join(root, "env"), "pig")
	onPath := fakeExecutable(t, filepath.Join(root, "path"), "pig")

	t.Setenv("PATH", filepath.Dir(onPath))
	t.Setenv(PigBinEnv, fromEnv)

	if got, err := ResolvePig(override); err != nil || got != override {
		t.Fatalf("override: %q, %v", got, err)
	}
	if got, err := ResolvePig(""); err != nil || got != fromEnv {
		t.Fatalf("$PIG_BIN: %q, %v", got, err)
	}
	t.Setenv(PigBinEnv, "")
	if got, err := ResolvePig(""); err != nil || got != onPath {
		t.Fatalf("PATH: %q, %v", got, err)
	}
}

func TestResolvePigNameOnPath(t *testing.T) {
	root := t.TempDir()
	named := fakeExecutable(t, filepath.Join(root, "bin"), "pig-nightly")
	t.Setenv("PATH", filepath.Dir(named))
	t.Setenv(PigBinEnv, "pig-nightly")
	if got, err := ResolvePig(""); err != nil || got != named {
		t.Fatalf("$PIG_BIN by name: %q, %v", got, err)
	}
	if got, err := ResolvePig("pig-nightly"); err != nil || got != named {
		t.Fatalf("override by name: %q, %v", got, err)
	}
}

func TestResolvePigBadOverride(t *testing.T) {
	_, err := ResolvePig(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "override") {
		t.Fatalf("err %v", err)
	}
}

func TestResolvePigErrorNamesEveryStrategy(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv(PigBinEnv, filepath.Join(empty, "nope"))
	got, err := ResolvePig("")
	if err == nil {
		// The test process itself runs under a pig ancestor.
		if !isPigExecutable(got) {
			t.Fatalf("resolved %q from nowhere", got)
		}
		t.Skipf("a pig ancestor exists: %s", got)
	}
	for _, want := range []string{"$PIG_BIN=", "not on PATH", "parent process", "Options.Pig"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
	t.Setenv(PigBinEnv, "")
	if _, err := ResolvePig(""); err == nil || !strings.Contains(err.Error(), "$PIG_BIN is unset") {
		t.Fatalf("err %v", err)
	}
}

func TestIsPigExecutable(t *testing.T) {
	cases := map[string]bool{
		"/usr/local/bin/pig":           true,
		"/opt/pig/pig (deleted)":       true,
		"C:\\tools\\pig.exe":           false,
		"pig.exe":                      true,
		"/usr/bin/piglet":              false,
		"/home/u/go/bin/rpc.test":      false,
		"/tmp/pig-bin/pig":             true,
		"/tmp/pig-bin/pig-nightly":     false,
		"/tmp/pig-bin/pig (deleted) x": false,
		"/tmp/pig-bin/pig(deleted)":    false,
	}
	for exe, want := range cases {
		if got := isPigExecutable(exe); got != want {
			t.Errorf("%q: %v, want %v", exe, got, want)
		}
	}
}
