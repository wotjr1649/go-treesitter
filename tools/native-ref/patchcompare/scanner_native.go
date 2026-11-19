package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

func scannerDump(exe, inputPath, dumpPath string, inputLen int, baseline bool) (scannerOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, stderr, failure, err := execScanner(ctx, exe, "dump", inputPath)
	if writeErr := write(dumpPath, data); writeErr != nil {
		return scannerOutcome{Status: "evidence-write-error"}, errors.Join(err, writeErr)
	}
	if len(stderr) != 0 {
		if writeErr := write(dumpPath+".stderr", stderr); writeErr != nil {
			return scannerOutcome{Status: "evidence-write-error"}, errors.Join(err, writeErr)
		}
	}
	if err != nil {
		if baseline && confirmedScannerCrash(failure) && len(data) == 0 {
			return scannerOutcome{Status: "baseline-crash", Crash: failure}, nil
		}
		return scannerOutcome{Status: failure}, err
	}
	t, err := parseScannerDump(data)
	if err != nil {
		return scannerOutcome{Status: "invalid-dump"}, err
	}
	out := scannerOutcome{Status: "tree", Tree: &t}
	if t.End > uint64(inputLen) || (!baseline && t.End != uint64(inputLen)) {
		return out, fmt.Errorf("root ends at %d, input has %d bytes", t.End, inputLen)
	}
	return out, nil
}

func execScanner(ctx context.Context, exe string, args ...string) (data, stderr []byte, failure string, err error) {
	cmd := proc.CommandContext(ctx, exe, args...)
	var output, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	if err := cmd.Start(); err != nil {
		return output.Bytes(), diagnostic.Bytes(), "start-error", err
	}
	err = cmd.Wait()
	if err != nil {
		switch {
		case ctx.Err() != nil:
			failure, err = "timeout", errors.Join(ctx.Err(), err)
		default:
			failure = "process-error"
			// A joined error also records a supervision failure and is never an expected crash.
			if exit, ok := err.(*exec.ExitError); ok {
				if kind := scannerCrash(exit.ProcessState); kind != "" {
					failure = kind
				}
			}
		}
	}
	return output.Bytes(), diagnostic.Bytes(), failure, err
}

func scannerCrash(p *os.ProcessState) string {
	if runtime.GOOS == "windows" {
		// Exit codes from confirmed Windows process exceptions, represented as NTSTATUS.
		switch uint32(p.ExitCode()) {
		case 0xc0000005:
			return "access-violation"
		case 0xc000001d:
			return "illegal-instruction"
		case 0xc000008e, 0xc0000094:
			return "arithmetic-fault"
		case 0xc00000fd:
			return "stack-overflow"
		}
		return ""
	}
	status, ok := p.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return ""
	}
	switch status.Signal() {
	case syscall.SIGSEGV:
		return "access-violation"
	case syscall.SIGBUS:
		return "bus-error"
	case syscall.SIGABRT:
		return "abort"
	case syscall.SIGILL:
		return "illegal-instruction"
	case syscall.SIGFPE:
		return "arithmetic-fault"
	}
	return ""
}

func confirmedScannerCrash(status string) bool {
	switch status {
	case "access-violation", "bus-error", "abort", "illegal-instruction", "arithmetic-fault", "stack-overflow":
		return true
	}
	return false
}

func parseScannerDump(b []byte) (tree, error) {
	header, body, found := bytes.Cut(b, []byte{'\n'})
	errFlag, count, valid := bytes.Cut(bytes.TrimPrefix(header, []byte("#tsg-dump/1 root_has_error=")), []byte(" nodes="))
	if !found || !bytes.HasPrefix(header, []byte("#tsg-dump/1 root_has_error=")) ||
		!valid || !(bytes.Equal(errFlag, []byte("0")) || bytes.Equal(errFlag, []byte("1"))) {
		return tree{}, fmt.Errorf("invalid native dump header")
	}
	n, err := strconv.ParseUint(string(count), 10, 32)
	if err != nil || n == 0 || n != uint64(bytes.Count(body, []byte{'\n'})) || !bytes.HasSuffix(body, []byte{'\n'}) {
		return tree{}, fmt.Errorf("invalid native dump node count")
	}
	root, rest, _ := bytes.Cut(body, []byte{'\n'})
	fields := bytes.Split(root, []byte{'\t'})
	if len(fields) != 12 || !bytes.Equal(fields[0], []byte("0")) ||
		!bytes.Equal(fields[1], []byte("-")) || !bytes.Equal(fields[7], errFlag) {
		return tree{}, fmt.Errorf("invalid native dump root row")
	}
	for len(rest) > 0 {
		row, next, _ := bytes.Cut(rest, []byte{'\n'})
		if bytes.Count(row, []byte{'\t'}) != 11 {
			return tree{}, fmt.Errorf("invalid native dump node row")
		}
		rest = next
	}
	end, err := strconv.ParseUint(string(fields[9]), 10, 32)
	if err != nil {
		return tree{}, fmt.Errorf("invalid native dump root end: %w", err)
	}
	return tree{HasError: errFlag[0] == '1', End: end, DumpSHA256: sha(b)}, nil
}
