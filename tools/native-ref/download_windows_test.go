package main

import (
	"net"
	"syscall"
	"testing"
)

func TestWindowsNetworkReset(t *testing.T) {
	for _, err := range []error{syscall.WSAECONNRESET, syscall.WSAECONNABORTED, syscall.ERROR_NETNAME_DELETED} {
		if !transientNetwork(&net.OpError{Op: "read", Net: "tcp", Err: err}) {
			t.Errorf("TCP error not retried: %v", err)
		}
	}
}
