//go:build !unix

package rpc

import (
	"os"
	"os/exec"
)

// setProcessGroup has no portable form; the child shares the parent's group.
func setProcessGroup(*exec.Cmd) {}

// killProcessGroup kills the child only; its descendants are not reached.
func killProcessGroup(process *os.Process) {
	if process == nil {
		return
	}
	_ = process.Kill()
}
