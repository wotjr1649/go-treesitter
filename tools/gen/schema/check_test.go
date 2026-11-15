package schema

import (
	"strings"
	"testing"
)

// Invalid-input literals and expected codes/verdicts are test data from
// wotjr1649/tree-sitter-grammar-kit src/kit/schema_test.go at 0a12156 (MIT).
// The reader and test runner were independently implemented for this repository.
func TestKitInvalidInputs(t *testing.T) {
	sub := func(extra string) string { return `[{"type":"a","named":true` + extra + `}]` }
	set := `{"multiple":false,"required":true,"types":[{"type":"a","named":true}]}`
	cases := []struct {
		name, doc, assess string
		codes             []string
	}{
		{"not-array", `{"type":"a","named":true}`, "FAIL", []string{"JSON_TYPE"}},
		{"null-document", `null`, "FAIL", []string{"JSON_NULL"}},
		{"zero-nodes", `[]`, "FAIL", []string{"SCHEMA_EMPTY"}},
		{"entry-not-object", `[1]`, "FAIL", []string{"JSON_TYPE"}},
		{"named-not-bool", `[{"type":"a","named":"yes"}]`, "FAIL", []string{"JSON_TYPE"}},
		{"type-not-string", `[{"type":7,"named":true}]`, "FAIL", []string{"JSON_TYPE"}},
		{"missing-type", `[{"named":true}]`, "FAIL", []string{"JSON_MISSING_FIELD"}},
		{"missing-named", `[{"type":"a"}]`, "FAIL", []string{"JSON_MISSING_FIELD"}},
		{"duplicate-node", `[{"type":"a","named":true},{"type":"a","named":true}]`, "FAIL", []string{"NODE_DUPLICATE"}},
		{"duplicate-node-other-shape", `[{"type":"?","named":false},{"type":"?","named":false,"fields":{}}]`, "FAIL", []string{"NODE_DUPLICATE"}},
		{"duplicate-field", sub(`,"fields":{"f":` + set + `,"f":` + set + `}`), "FAIL", []string{"JSON_DUPLICATE_KEY"}},
		{"escaped-duplicate-field", sub(`,"fields":{"f":` + set + `,"` + "\\" + `u0066":` + set + `}`), "FAIL", []string{"JSON_DUPLICATE_KEY"}},
		{"field-value-null", sub(`,"fields":{"f":null}`), "FAIL", []string{"JSON_NULL"}},
		{"field-value-number", sub(`,"fields":{"f":1,"g":` + set + `}`), "FAIL", []string{"JSON_TYPE"}},
		{"children-number", sub(`,"children":1`), "FAIL", []string{"JSON_TYPE"}},
		{"subtypes-object", sub(`,"subtypes":{}`), "FAIL", []string{"JSON_TYPE"}},
		{"member-null", sub(`,"children":{"multiple":false,"required":true,"types":[null]}`), "FAIL", []string{"JSON_NULL"}},
		{"duplicate-member", sub(`,"children":{"multiple":false,"required":true,"types":[{"type":"a","named":true},{"type":"a","named":true}]}`), "FAIL", []string{"MEMBER_DUPLICATE"}},
		{"unsupported-key", sub(`,"visible":true`), "BLOCKED", []string{"SCHEMA_KEY_UNSUPPORTED"}},
		{"unsupported-member-key", sub(`,"children":{"multiple":false,"required":true,"types":[{"type":"a","named":true,"alias":true}]}`), "BLOCKED", []string{"SCHEMA_KEY_UNSUPPORTED"}},
		{"unsupported-and-invalid", `[{"type":"a","named":true,"visible":true},{"type":"a","named":true}]`, "FAIL", []string{"SCHEMA_KEY_UNSUPPORTED", "NODE_DUPLICATE"}},
		{"broken-subtype", `[{"type":"s","named":true,"subtypes":[{"type":"missing","named":true}]}]`, "FAIL", []string{"REFERENCE_UNRESOLVED"}},
		{"subtype-named-mismatch", `[{"type":"a","named":false},{"type":"s","named":true,"subtypes":[{"type":"a","named":true}]}]`, "FAIL", []string{"REFERENCE_UNRESOLVED"}},
		{"cycle", `[{"type":"s","named":true,"subtypes":[{"type":"t","named":true}]},{"type":"t","named":true,"subtypes":[{"type":"s","named":true}]}]`, "FAIL", []string{"SUPERTYPE_CYCLE"}},
		{"self-cycle", `[{"type":"s","named":true,"subtypes":[{"type":"s","named":true}]}]`, "FAIL", []string{"SUPERTYPE_CYCLE"}},
		{"types-empty", sub(`,"fields":{"f":{"multiple":false,"required":true,"types":[]}}`), "FAIL", []string{"TYPES_EMPTY"}},
		{"subtypes-empty", sub(`,"subtypes":[]`), "FAIL", []string{"SUBTYPES_EMPTY"}},
		{"supertype-with-fields", `[{"type":"a","named":true},{"type":"s","named":true,"fields":{},"subtypes":[{"type":"a","named":true}]}]`, "FAIL", []string{"SUPERTYPE_SHAPE"}},
		{"two-roots", `[{"type":"a","named":true,"root":true},{"type":"b","named":true,"root":true}]`, "FAIL", []string{"ROOT_MULTIPLE"}},
		{"field-name-empty", sub(`,"fields":{"":` + set + `}`), "FAIL", []string{"FIELD_NAME_EMPTY"}},
		{"fields-null", sub(`,"fields":null`), "FAIL", []string{"JSON_NULL"}},
		{"children-null", sub(`,"children":null`), "FAIL", []string{"JSON_NULL"}},
		{"root-null", sub(`,"root":null`), "FAIL", []string{"JSON_NULL"}},
		{"set-missing-required", sub(`,"children":{"multiple":false,"types":[{"type":"a","named":true}]}`), "FAIL", []string{"JSON_MISSING_FIELD"}},
		{"set-empty-object", sub(`,"children":{}`), "FAIL", []string{"JSON_MISSING_FIELD"}},
		{"trailing-value", sub(``) + ` []`, "FAIL", []string{"JSON_TRAILING_VALUE"}},
		{"truncated", sub(``)[:10], "FAIL", []string{"JSON_TRUNCATED"}},
		{"invalid-utf8", "[{\"type\":\"\xff\",\"named\":true}]", "FAIL", []string{"JSON_INVALID_UTF8"}},
	}
	if len(cases) != 38 {
		t.Fatalf("got %d kit cases, want 38", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Check([]byte(tc.doc))
			if got.Assessment != tc.assess || got.Nodes != nil {
				t.Fatalf("got %+v, want %s with no nodes", got, tc.assess)
			}
			for _, code := range tc.codes {
				if !findingCode(got, code) {
					t.Errorf("missing %s: %+v", code, got.Findings)
				}
			}
		})
	}
}

func findingCode(r Result, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestStrictKeysAndUnicode(t *testing.T) {
	for _, doc := range []string{
		`[{"Type":"a","named":true}]`,
		`[{"type":"a","Named":true}]`,
		`[{"type":"a","named":true,"children":{"Multiple":false,"required":false,"types":[{"type":"a","named":true}]}}]`,
	} {
		r := Check([]byte(doc))
		if r.Assessment != "FAIL" || !findingCode(r, "SCHEMA_KEY_UNSUPPORTED") {
			t.Errorf("case-folded key accepted: %+v", r)
		}
	}
	for _, doc := range []string{
		`[{"type":"a","named":true,"t\u0079pe":"b"}]`,
		`[{"type":"a","named":true,"children":{"multiple":false,"required":false,"types":[{"type":"a","named":true,"\u006eamed":true}]}}]`,
	} {
		r := Check([]byte(doc))
		if r.Assessment != "FAIL" || !findingCode(r, "JSON_DUPLICATE_KEY") {
			t.Errorf("escaped duplicate accepted: %+v", r)
		}
	}
	for _, doc := range []string{
		`[{"type":"","named":true}]`,
		`[{"type":"한글😀","named":true}]`,
		`[{"type":"a","named":true},{"type":"a","named":false,"fields":{}}]`,
		`[{"type":"a","named":true,"fields":{"alias":{"multiple":false,"required":false,"types":[{"type":"undeclared","named":true}]}}}]`,
	} {
		r := Check([]byte(doc))
		if r.Assessment != "PASS" {
			t.Errorf("valid convention failed: %+v", r)
		}
	}
	r := Check([]byte(`[{"type":"a","named":true,"fields":{"a/b~c":{"multiple":false,"required":false,"types":[{"type":"undeclared","named":true}]}}}]`))
	if len(r.Findings) != 1 || r.Findings[0].Severity != "warning" || r.Findings[0].Path != "/0/fields/a~1b~0c/types/0" {
		t.Errorf("warning pointer: %+v", r)
	}
}

func TestPresenceAndDeterminism(t *testing.T) {
	r := Check([]byte(`[{"type":"a","named":true},{"type":"b","named":true,"root":false,"extra":false,"fields":{}}]`))
	if r.Assessment != "PASS" || r.Nodes[0].Fields != nil || r.Nodes[0].Root != nil || r.Nodes[1].Fields == nil || r.Nodes[1].Root == nil || *r.Nodes[1].Root || r.Nodes[1].Extra == nil || *r.Nodes[1].Extra {
		t.Fatalf("presence lost: %+v", r)
	}
	doc := `[{"type":"a","named":true,"fields":{"z":null,"a":null},"Z":true,"A":true}]`
	for i := 0; i < 20; i++ {
		got := Check([]byte(doc))
		if got.Assessment != "FAIL" || len(got.Findings) != 4 || got.Findings[0].Path != "/0/A" || got.Findings[1].Path != "/0/Z" || got.Findings[2].Path != "/0/fields/a" {
			t.Fatalf("unstable findings: %+v", got)
		}
	}
	deep := strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66)
	if got := Check([]byte(deep)); got.Assessment != "FAIL" || !findingCode(got, "JSON_DEPTH_LIMIT") {
		t.Fatalf("depth: %+v", got)
	}
}

func TestSurrogateEscapes(t *testing.T) {
	for _, value := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\u0041`} {
		r := Check([]byte(`[{"type":"` + value + `","named":true}]`))
		if r.Assessment != "FAIL" || !findingCode(r, "JSON_INVALID_UNICODE") {
			t.Errorf("accepted %s: %+v", value, r)
		}
	}
	for _, value := range []string{`\ud83d\ude00`, `\\ud800`, `\ufffd`} {
		r := Check([]byte(`[{"type":"` + value + `","named":true}]`))
		if r.Assessment != "PASS" {
			t.Errorf("rejected %s: %+v", value, r)
		}
	}
}
