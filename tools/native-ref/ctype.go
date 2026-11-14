package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// buildCtype compiles unmodified pinned sources against private 32-bit character headers.
func buildCtype(s source, dl, work, here, cc, opt string, fetch bool) ([]string, error) {
	s.Dirs = []string{"libc-top-half/musl/src/ctype"}
	if err := prepare(s, dl, work, fetch, map[string]bool{}); err != nil {
		return nil, err
	}
	var objects []string
	for _, name := range []string{"iswalnum", "iswalpha", "iswdigit", "iswspace", "towctrans", "isdigit"} {
		obj := filepath.Join(work, "tsg_"+name+".o")
		args := []string{"-std=c11", opt, "-fno-builtin", "-I" + filepath.Join(here, "ctype"), "-c",
			filepath.Join(work, s.ExtractDir, "libc-top-half", "musl", "src", "ctype", name+".c"), "-o", obj}
		cmd := exec.Command(cc, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		objects = append(objects, obj)
	}
	return objects, nil
}
