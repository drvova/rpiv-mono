package rpc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PigBinEnv is the environment variable that names the pig executable when
// the caller gives no override.
const PigBinEnv = "PIG_BIN"

// pigBaseNames are the executable base names that identify a pig process in
// the ancestor walk.
var pigBaseNames = []string{"pig", "pig.exe"}

// ResolvePig returns the pig executable a child session runs. The strategies
// are tried in order and the first usable one wins:
//
//  1. override, when not empty. A name without a path separator is looked up
//     on PATH; anything else is used as a path. An override that does not
//     resolve is an error on its own, since it was chosen deliberately.
//  2. $PIG_BIN, with the same lookup rule.
//  3. "pig" on PATH.
//  4. On Linux, the chain of parent processes: the first ancestor whose
//     executable (/proc/<pid>/exe) is named pig. This is how an extension
//     running under pig finds the very binary that is hosting it.
//
// The returned error names every strategy and why it failed.
func ResolvePig(override string) (string, error) {
	if override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return "", fmt.Errorf("rpc: pig executable override %q is not usable: %w", override, err)
		}
		return path, nil
	}
	var reasons []string
	if fromEnv := os.Getenv(PigBinEnv); fromEnv != "" {
		path, err := exec.LookPath(fromEnv)
		if err == nil {
			return path, nil
		}
		reasons = append(reasons, fmt.Sprintf("$%s=%q is not usable (%v)", PigBinEnv, fromEnv, err))
	} else {
		reasons = append(reasons, fmt.Sprintf("$%s is unset", PigBinEnv))
	}
	path, err := exec.LookPath("pig")
	if err == nil {
		return path, nil
	}
	reasons = append(reasons, fmt.Sprintf("\"pig\" is not on PATH (%v)", err))
	path, err = pigFromAncestors()
	if err == nil {
		return path, nil
	}
	reasons = append(reasons, err.Error())
	return "", fmt.Errorf("rpc: cannot find the pig executable: %s; set Options.Pig, export %s, or put pig on PATH", strings.Join(reasons, "; "), PigBinEnv)
}

// errNoPigAncestor reports an ancestor walk that found no pig.
var errNoPigAncestor = errors.New("no parent process is named pig")

// isPigExecutable reports whether an executable path names pig. A deleted or
// replaced binary keeps its link text with a " (deleted)" suffix, which still
// identifies it.
func isPigExecutable(exe string) bool {
	base := strings.TrimSuffix(filepath.Base(exe), " (deleted)")
	for _, name := range pigBaseNames {
		if base == name {
			return true
		}
	}
	return false
}
