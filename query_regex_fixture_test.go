package treesitter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"unicode"
)

// E3: recorded CLI outputs, not a native C reference comparison.
func TestQueryRegexRecordedCLI(t *testing.T) {
	data, err := os.ReadFile("testdata/query-regex-cli.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		CLIVersion string `json:"cli_version"`
		CLISHA256  string `json:"cli_sha256"`
		Inputs     []struct {
			Name, Base64 string
			CaptureStart int  `json:"capture_start_byte"`
			CaptureEnd   *int `json:"capture_end_byte"`
		}
		Patterns []struct {
			Name        string
			Pattern     string `json:"pattern_base64"`
			Matches     []bool
			GoOverrides map[string]bool `json:"go_overrides"`
			Residual    string
		}
		Acceptance []struct {
			Name     string
			Pattern  string `json:"pattern_base64"`
			GoReject string `json:"go_reject"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.CLIVersion != "tree-sitter 0.27.2" || len(fixture.CLISHA256) != 64 || len(fixture.Patterns) == 0 || len(fixture.Inputs) == 0 {
		t.Fatal("missing CLI fixture identity or matrix")
	}
	seen := map[string]bool{}
	checkName := func(category, name string) {
		key := category + "/" + strings.ToLower(name)
		if seen[key] {
			t.Fatalf("case-insensitive fixture filename collision: %s", key)
		}
		seen[key] = true
	}
	for _, input := range fixture.Inputs {
		checkName("input", input.Name)
	}
	for _, row := range fixture.Patterns {
		checkName("pattern", row.Name)
	}
	for _, probe := range fixture.Acceptance {
		checkName("accept", probe.Name)
	}
	decode := func(t *testing.T, value string) string {
		t.Helper()
		b, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, row := range fixture.Patterns {
		t.Run(row.Name, func(t *testing.T) {
			pattern := decode(t, row.Pattern)
			re, err := compileQueryRegex(pattern)
			if err != nil {
				t.Fatal(err)
			}
			compile := newQueryMeter(context.Background(), math.MaxInt64, "query regex")
			budgeted, err := compileQueryRegexWithMeter(pattern, &compile)
			if err != nil {
				t.Fatal(err)
			}
			if len(row.Matches) != len(fixture.Inputs) {
				t.Fatal("incomplete matrix row")
			}
			for i, input := range fixture.Inputs {
				text := decode(t, input.Base64)
				end := len(text)
				if input.CaptureEnd != nil {
					end = *input.CaptureEnd
				}
				if input.CaptureStart < 0 || end < input.CaptureStart || end > len(text) {
					t.Fatal("invalid recorded capture window")
				}
				// The CLI predicate sees the document capture, excluding leading/trailing whitespace.
				text = text[input.CaptureStart:end]
				want := row.Matches[i]
				if override, ok := row.GoOverrides[input.Name]; ok {
					if row.Residual == "" {
						t.Fatal("unexplained fixture residual")
					}
					want = override
					if row.Name == "property-L" && input.Name == "unicode17-new-letter" {
						want = unicode.IsLetter('\U00010940')
					}
				}
				work := newQueryMeter(context.Background(), math.MaxInt64, "query predicate work")
				got := regexVMMatchWithMeter(budgeted.prog, text, budgeted.asciiBoundaries, &work)
				if work.err != nil || got != want || budgeted.MatchString(text) != want {
					t.Errorf("%s: budgeted regex=%v err=%v want=%v", input.Name, got, work.err, want)
				}
				if got := re.MatchString(text); got != want {
					t.Errorf("%s: pattern=%q Go=%v recorded CLI=%v expected Go=%v residual=%q", input.Name, pattern, got, row.Matches[i], want, row.Residual)
				}
			}
		})
	}
	for _, probe := range fixture.Acceptance {
		t.Run("accept/"+probe.Name, func(t *testing.T) {
			_, err := compileQueryRegex(decode(t, probe.Pattern))
			if (err != nil) != (probe.GoReject != "") {
				t.Errorf("expected rejection=%q, got %v", probe.GoReject, err)
			}
		})
	}
}
