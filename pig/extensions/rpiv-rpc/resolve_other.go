//go:build !linux

package rpc

import (
	"fmt"
	"runtime"
)

// pigFromAncestors is Linux-only: other systems have no portable way to read
// an ancestor's executable path.
func pigFromAncestors() (string, error) {
	return "", fmt.Errorf("%w (the parent-process lookup is not available on %s)", errNoPigAncestor, runtime.GOOS)
}
