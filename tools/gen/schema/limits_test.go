package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFindingLimit(t *testing.T) {
	for _, count := range []int{1000, 1001, 2000} {
		r := Check([]byte("[" + strings.Repeat("0,", count-1) + "0]"))
		want := count
		if want > 1000 {
			want = 1001
		}
		if r.Assessment != "FAIL" || r.Nodes != nil || len(r.Findings) != want {
			t.Fatalf("count=%d assessment=%s nodes=%d findings=%d", count, r.Assessment, len(r.Nodes), len(r.Findings))
		}
		if count > 1000 {
			last := r.Findings[1000]
			if last.Code != "FINDINGS_TRUNCATED" || last.Severity != "error" || last.Message != fmt.Sprintf("retained first 1000 of %d findings", count) {
				t.Fatalf("summary: %+v", last)
			}
		} else if findingCode(r, "FINDINGS_TRUNCATED") {
			t.Fatal("unexpected truncation")
		}
		b, err := json.Marshal(r)
		if err != nil || len(b) > 256<<10 {
			t.Fatalf("output=%d err=%v", len(b), err)
		}
	}
	var doc strings.Builder
	doc.WriteByte('[')
	for i := 0; i < 1001; i++ {
		if i > 0 {
			doc.WriteByte(',')
		}
		fmt.Fprintf(&doc, `{"type":"n%d","named":true,"unknown":0}`, i)
	}
	r := Check([]byte(doc.String() + "]"))
	if r.Assessment != "BLOCKED" || len(r.Findings) != 1001 || r.Findings[1000].Message != "retained first 1000 of 1001 findings" {
		t.Fatalf("blocked findings: %+v", r)
	}
	// A failure after the retained BLOCKED findings must still take precedence.
	r = Check([]byte(doc.String() + ",null]"))
	if r.Assessment != "FAIL" || r.Findings[1000].Message != "retained first 1000 of 1002 findings" {
		t.Fatalf("late failure: %+v", r)
	}
	// Distinct fields permit repeated undeclared refs without MEMBER_DUPLICATE.
	doc.Reset()
	doc.WriteString(`[{"type":"a","named":true,"fields":{`)
	for i := 0; i < 1001; i++ {
		if i > 0 {
			doc.WriteByte(',')
		}
		fmt.Fprintf(&doc, `"f%d":{"multiple":false,"required":false,"types":[{"type":"missing","named":true}]}`, i)
	}
	r = Check([]byte(doc.String() + "}}]"))
	if r.Assessment != "FAIL" || r.Nodes != nil || len(r.Findings) != 1001 || !findingCode(r, "FINDINGS_TRUNCATED") {
		t.Fatalf("warning truncation: %+v", r)
	}
	// Even when earlier entries failed, syntax errors retain their single reader finding.
	r = Check([]byte("[" + strings.Repeat("0,", 2000)))
	if r.Assessment != "FAIL" || len(r.Findings) != 1 || r.Findings[0].Code != "JSON_TRUNCATED" {
		t.Fatalf("syntax precedence: %+v", r)
	}
}

func FuzzCheck(f *testing.F) {
	for _, doc := range []string{
		`[{"type":"a","named":true}]`,
		`[{"type":"a","named":true,"children":{"multiple":false,"required":true,"types":[{"type":"a","named":true}]}}]`,
		`[{"type":"a","named":true`, `[{"type":"a","type":"b","named":true}]`,
		`[{"type":"\ud800","named":true}]`, `[{"type":"가😀","named":true}]`,
		"[" + strings.Repeat("0,", 1000) + "0]",
		strings.Repeat("[", 65) + strings.Repeat("]", 65),
	} {
		f.Add([]byte(doc))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := Check(data)
		if len(r.Findings) > 1001 || !reflect.DeepEqual(r, Check(data)) {
			t.Fatal("unbounded or unstable result")
		}
		errors := 0
		for _, finding := range r.Findings {
			if finding.Severity == "error" {
				errors++
			}
		}
		switch r.Assessment {
		case "PASS":
			if errors != 0 || len(r.Nodes) == 0 || !utf8.Valid(data) || !json.Valid(data) || len(Diff(r.Nodes, r.Nodes)) != 0 {
				t.Fatal("invalid PASS")
			}
		case "FAIL", "BLOCKED":
			if r.Nodes != nil || errors == 0 {
				t.Fatal("invalid non-PASS")
			}
		default:
			t.Fatalf("unknown assessment %q", r.Assessment)
		}
	})
}

func TestReaderFixes(t *testing.T) {
	for _, doc := range []string{`[{"type":"a","named":true`, `[{"type":"`, `[0,{"type":"a"`, `[0,`} {
		r := Check([]byte(doc))
		if r.Assessment != "FAIL" || len(r.Findings) != 1 || r.Findings[0].Code != "JSON_TRUNCATED" {
			t.Errorf("%q: %+v", doc, r)
		}
	}
	for _, tc := range []struct{ doc, absent string }{
		{`[{"type":"a","named":true,"subtypes":[{"type":5,"named":true}]}]`, "SUBTYPES_EMPTY"},
		{`[{"type":"a","named":true,"children":{"multiple":false,"required":false,"types":[{"type":1}]}}]`, "TYPES_EMPTY"},
	} {
		r := Check([]byte(tc.doc))
		if r.Assessment != "FAIL" || !findingCode(r, "JSON_TYPE") || findingCode(r, tc.absent) {
			t.Errorf("noise: %+v", r)
		}
	}
	for _, leaf := range []string{"", "0"} {
		for _, depth := range []int{63, 64, 65} {
			r := Check([]byte(strings.Repeat("[", depth) + leaf + strings.Repeat("]", depth)))
			if findingCode(r, "JSON_DEPTH_LIMIT") != (depth > 64) {
				t.Errorf("depth=%d leaf=%q: %+v", depth, leaf, r)
			}
		}
	}
	for _, key := range []string{strings.Repeat("k", 1024), strings.Repeat("k", 1025), strings.Repeat("가", 341), strings.Repeat("가", 342)} {
		r := Check([]byte(`[{"type":"a","named":true,"` + key + `":0}]`))
		if findingCode(r, "JSON_KEY_LIMIT") != (len(key) > 1024) {
			t.Errorf("key bytes=%d: %+v", len(key), r)
		}
	}
	r := Check([]byte(`[{"type":"a","named":true,"fields":{"a/b~c":null}}]`))
	if len(r.Findings) != 1 || r.Findings[0].Path != "/0/fields/a~1b~0c" {
		t.Fatalf("pointer: %+v", r)
	}
}
