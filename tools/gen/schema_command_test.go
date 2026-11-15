package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaCommand(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{}
	for name, doc := range map[string]string{
		"old": `[{"type":"a","named":true}]`,
		"new": `[{"type":"a","named":true,"fields":{}}]`,
		"bad": `[{"type":"a","named":true,"Type":"b"}]`,
	} {
		paths[name] = filepath.Join(dir, name+".json")
		if err := os.WriteFile(paths[name], []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		args       []string
		assessment string
		wantError  bool
		changes    int
	}{
		{[]string{"check", paths["old"]}, "PASS", false, 0},
		{[]string{"check", paths["bad"]}, "BLOCKED", true, 0},
		{[]string{"diff", paths["old"], paths["new"]}, "FAIL", false, 1},
		{[]string{"diff", paths["old"], paths["old"]}, "PASS", false, 0},
		{[]string{"diff", paths["old"], paths["bad"]}, "BLOCKED", true, 0},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := runSchemaCommand(tc.args, &out)
			if (err != nil) != tc.wantError {
				t.Fatalf("error %v", err)
			}
			var r schemaReport
			if err := json.Unmarshal(out.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			if r.Assessment != tc.assessment || !r.StaticOnly || len(r.Differences) != tc.changes {
				t.Fatalf("report: %s", out.String())
			}
			input := r.Input
			if input == nil {
				input = r.Baseline
			}
			data, err := os.ReadFile(input.Name)
			if err != nil || input.SHA256 != sumBytes(data) || input.Bytes != len(data) {
				t.Fatalf("input identity: %+v, %v", input, err)
			}
		})
	}
	for _, args := range [][]string{nil, {"check"}, {"diff", "a"}, {"other"}, {"check", "a", "b"}, {"check", filepath.Join(dir, "absent")}} {
		var out bytes.Buffer
		if err := runSchemaCommand(args, &out); err == nil || out.Len() != 0 {
			t.Fatalf("bad arguments %v: %v, %s", args, err, out.String())
		}
	}
	if err := runSchemaCommand([]string{"check", paths["old"]}, failedSchemaWriter{}); err == nil {
		t.Fatal("ignored output error")
	}
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, bytes.Repeat([]byte{' '}, maxSchemaBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSchema(big); err == nil {
		t.Fatal("accepted oversized input")
	}
}

type failedSchemaWriter struct{}

func (failedSchemaWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }
