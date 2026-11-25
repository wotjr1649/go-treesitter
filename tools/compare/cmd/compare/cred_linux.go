//go:build linux

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// setCredential makes the child run as uid:uid with no supplementary groups. It needs CAP_SETUID/CAP_SETGID, which
// the sandbox container grants to the measurer only; the child itself starts with no capabilities.
func setCredential(cmd *exec.Cmd, uid int) error {
	if uid <= 0 {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(uid), Groups: []uint32{}}
	return nil
}

func chown(path string, uid int) error { return os.Chown(path, uid, uid) }
