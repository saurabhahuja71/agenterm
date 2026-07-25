//go:build unix

package tools

import (
	"os"
	"syscall"
)

func shellSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killShellProcess(p *os.Process) {
	if p == nil {
		return
	}
	// Negative PID: kill process group started with Setpgid.
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
}
