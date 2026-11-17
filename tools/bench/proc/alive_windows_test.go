package proc

import (
	"errors"
	"syscall"
)

func processAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return !errors.Is(err, syscall.Errno(87)) // ERROR_INVALID_PARAMETER means PID no longer exists.
	}
	defer syscall.CloseHandle(handle)
	status, err := syscall.WaitForSingleObject(handle, 0)
	return err != nil || status == syscall.WAIT_TIMEOUT
}
