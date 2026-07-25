//go:build windows

package tools

import (
	"os"
	"syscall"
)

func shellSysProcAttr() *syscall.SysProcAttr {
	return nil
}

func killShellProcess(p *os.Process) {
	if p == nil {
		return
	}
	_ = p.Kill()
}
