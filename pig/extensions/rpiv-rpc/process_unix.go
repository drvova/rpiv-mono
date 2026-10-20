//go:build unix

package rpc

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the child the leader of a new process group, so that
// killProcessGroup reaches the tools and extension processes it spawned.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the child's process group, then to the
// child itself in case it changed groups. Errors are ignored: the process may
// already be gone.
func killProcessGroup(process *os.Process) {
	if process == nil {
		return
	}
	_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
	_ = process.Kill()
}
