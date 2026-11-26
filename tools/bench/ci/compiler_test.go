package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for the C compiler: BENCHCI_FAKE_CC=ok prints a gcc-like --version and -dumpmachine.
func TestMain(m *testing.M) {
	switch os.Getenv("BENCHCI_FAKE_CC") {
	case "ok":
		if len(os.Args) > 1 && os.Args[1] == "-dumpmachine" {
			fmt.Print("aarch64-w64-windows-gnu\r\n")
		} else {
			fmt.Print("clang version 23.1.5\r\nTarget: aarch64-w64-windows-gnu\r\n")
		}
		os.Exit(0)
	case "fail":
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func TestCompilerIdentity(t *testing.T) {
	if got := compilerIdentity(""); !strings.HasPrefix(got, "not recorded") {
		t.Errorf("empty -cc: %q", got)
	}
	t.Setenv("BENCHCI_FAKE_CC", "ok")
	if got, want := compilerIdentity(os.Args[0]), "`clang version 23.1.5`, target `aarch64-w64-windows-gnu`"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	t.Setenv("BENCHCI_FAKE_CC", "fail")
	if got := compilerIdentity(os.Args[0]); !strings.Contains(got, "query failed") {
		t.Errorf("failing compiler: %q", got)
	}
}
