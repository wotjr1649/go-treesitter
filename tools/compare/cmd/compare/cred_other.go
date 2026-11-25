//go:build !linux

package main

import (
	"errors"
	"os/exec"
)

func setCredential(_ *exec.Cmd, uid int) error {
	if uid > 0 {
		return errors.New("per-candidate users need Linux")
	}
	return nil
}

func chown(string, int) error { return errors.New("per-candidate users need Linux") }
