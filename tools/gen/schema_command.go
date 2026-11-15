package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wotjr1649/go-treesitter/tools/gen/schema"
)

const maxSchemaBytes = 16 << 20

type schemaInput struct {
	Name   string        `json:"name"`
	Bytes  int           `json:"bytes"`
	SHA256 string        `json:"sha256"`
	Check  schema.Result `json:"check"`
}

type schemaReport struct {
	Operation   string          `json:"operation"`
	StaticOnly  bool            `json:"staticOnly"`
	Assessment  string          `json:"assessment"`
	Input       *schemaInput    `json:"input,omitempty"`
	Baseline    *schemaInput    `json:"baseline,omitempty"`
	Candidate   *schemaInput    `json:"candidate,omitempty"`
	Differences []schema.Change `json:"differences,omitempty"`
}

func readSchema(name string) (*schemaInput, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSchemaBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSchemaBytes {
		return nil, fmt.Errorf("%q: schema exceeds %d bytes", name, maxSchemaBytes)
	}
	return &schemaInput{Name: name, Bytes: len(b), SHA256: sumBytes(b), Check: schema.Check(b)}, nil
}

// Changes are review evidence, so only invalid inputs or I/O errors fail the command.
func runSchemaCommand(args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "check" && args[0] != "diff") ||
		(args[0] == "check" && len(args) != 2) || (args[0] == "diff" && len(args) != 3) {
		return fmt.Errorf("usage: gen schema check FILE | gen schema diff OLD NEW")
	}
	a, err := readSchema(args[1])
	if err != nil {
		return err
	}
	r := schemaReport{Operation: "schema " + args[0], StaticOnly: true, Assessment: a.Check.Assessment}
	invalid := a.Check.Assessment != "PASS"
	if args[0] == "check" {
		r.Input = a
	} else {
		b, err := readSchema(args[2])
		if err != nil {
			return err
		}
		r.Baseline, r.Candidate = a, b
		invalid = invalid || b.Check.Assessment != "PASS"
		if invalid {
			r.Assessment = "BLOCKED"
			if a.Check.Assessment == "FAIL" || b.Check.Assessment == "FAIL" {
				r.Assessment = "FAIL"
			}
		} else {
			r.Differences = schema.Diff(a.Check.Nodes, b.Check.Nodes)
			if len(r.Differences) > 0 {
				r.Assessment = "FAIL"
			}
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("invalid or unsupported schema input; see JSON findings")
	}
	return nil
}
