package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The copied test executable acts as the native command to test the complete data-writing path.
func TestMain(m *testing.M) {
	if mode := os.Getenv("TSG_CICONF_TEST_NATIVE"); mode != "" {
		if len(os.Args) == 7 && os.Args[1] == "edit-fields" {
			if mode == "fail" {
				os.Exit(4)
			}
			for _, path := range os.Args[2:4] {
				if _, err := os.ReadFile(path); err != nil {
					os.Exit(3)
				}
			}
			for _, offset := range os.Args[4:] {
				if _, err := strconv.Atoi(offset); err != nil {
					os.Exit(2)
				}
			}
			fmt.Print("#tsg-fields/1 nodes=2 fields=1\n0\tvalue\t1\n")
			os.Exit(0)
		}
		if len(os.Args) > 1 && (os.Args[1] == "dump" || os.Args[1] == "edit" || os.Args[1] == "changed-ranges") {
			os.Exit(0)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestEditedFieldsResults(t *testing.T) {
	dir := t.TempDir()
	pins := filepath.Join(dir, "pins.json")
	if err := writeFile(pins, []byte(`{"grammars":{"json":{"extractDir":"json"}}}`)); err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join(dir, "corpus")
	if err := writeFile(filepath.Join(corpus, "json", "test", "corpus", "sample.txt"), []byte("===\nsample\n===\n{\"a\":1}\n---\n(document)\n")); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "exe", "json")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if err := writeFile(exe, binary); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(exe, 0o755); err != nil {
		t.Fatal(err)
	}
	readRows := func(path string) [][]string {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rows, err := csv.NewReader(f).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	for _, mode := range []string{"ok", "fail"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TSG_CICONF_TEST_NATIVE", mode)
			out := filepath.Join(dir, mode)
			runErr := run(pins, corpus, filepath.Dir(exe), out, 1, 1, "json", false)
			if (runErr != nil) != (mode == "fail") {
				t.Fatalf("run = %v", runErr)
			}
			rows := readRows(filepath.Join(out, "edits", "results-native.csv"))
			if len(rows) != 4 || len(rows[0]) != 8 || rows[0][6] != "edit_fields_exit" || rows[0][7] != "edit_fields_sha256" {
				t.Fatalf("unexpected native results: %v", rows)
			}
			for _, row := range rows[1:] {
				path := filepath.Join(out, "edits", filepath.FromSlash(row[0])+".fields")
				fields, err := os.ReadFile(path)
				if mode == "ok" {
					if err != nil || string(fields) != "#tsg-fields/1 nodes=2 fields=1\n0\tvalue\t1\n" || row[6] != "0" || row[7] != sha(fields) {
						t.Fatalf("%s: fields=%q, error=%v, result=%v", path, fields, err, row)
					}
				} else if !os.IsNotExist(err) || row[6] != "4" {
					t.Fatalf("failed native result written as success: %v, %v", row, err)
				}
			}
			if mode == "fail" {
				fails := readRows(filepath.Join(out, "native-failures.csv"))
				if len(fails) != 4 {
					t.Fatalf("native failures = %v", fails)
				}
				for _, f := range fails[1:] {
					if f[1] != "edit-fields" || !strings.Contains(runErr.Error(), "3 native reference runs failed") {
						t.Fatalf("failure not retained: %v, %v", f, runErr)
					}
				}
			}
		})
	}
}
