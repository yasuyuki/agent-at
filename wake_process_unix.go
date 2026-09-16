//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
)

func startWakeProcess(cmd *exec.Cmd) (terminate func() error, release func(), err error) {
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	attr.Setpgid = true
	attr.Pgid = 0
	cmd.SysProcAttr = &attr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	var once sync.Once
	var killErr error
	terminate = func() error {
		once.Do(func() {
			killErr = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if errors.Is(killErr, syscall.ESRCH) {
				killErr = nil
			}
		})
		return killErr
	}
	return terminate, func() { _ = terminate() }, nil
}
