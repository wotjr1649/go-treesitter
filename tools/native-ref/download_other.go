//go:build !windows

package main

func transientWindowsNetwork(error) bool { return false }
