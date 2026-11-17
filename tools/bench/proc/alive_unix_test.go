//go:build !windows

package proc

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processAlive(pid int) bool {
	// Linux's reparented zombie is terminated, although kill(pid, 0) still finds it.
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if end := strings.LastIndexByte(string(data), ')'); end >= 0 && len(data) > end+2 && data[end+2] == 'Z' {
			return false
		}
	}
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
