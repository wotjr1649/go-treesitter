package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Fixture documents and expected lists are test data from tree-sitter-grammar-kit
// 0a12156, src/kit/schema_test.go, Copyright (c) 2026 tree-sitter-grammar-kit contributors, MIT.
const schemaBase = `[
 {"type":"program","named":true,"root":true,"fields":{},"children":{"multiple":true,"required":false,"types":[{"type":"statement","named":true}]}},
 {"type":"expression","named":true,"subtypes":[{"type":"call","named":true},{"type":"identifier","named":true}]},
 {"type":"statement","named":true,"fields":{"value":{"multiple":false,"required":true,"types":[{"type":";","named":false},{"type":"expression","named":true}]}}},
 {"type":"call","named":true,"fields":{"arguments":{"multiple":true,"required":false,"types":[{"type":"expression","named":true}]},"function":{"multiple":false,"required":true,"types":[{"type":"identifier","named":true}]}}},
 {"type":"identifier","named":true},
 {"type":"comment","named":true,"extra":true},
 {"type":"null","named":true},
 {"type":"null","named":false},
 {"type":"this","named":false},
 {"type":";","named":false}
]`

const schemaCandidate = `[
 {"type":"program","named":true,"fields":{},"children":{"multiple":false,"required":true,"types":[{"type":"expression","named":true},{"type":"statement","named":true}]}},
 {"type":"expression","named":true,"subtypes":[{"type":"identifier","named":true},{"type":"number","named":true}]},
 {"type":"statement","named":true,"fields":{"value":{"multiple":false,"required":true,"types":[{"type":"expression","named":true},{"type":"number","named":true}]}},"children":{"multiple":false,"required":false,"types":[{"type":"identifier","named":true}]}},
 {"type":"call","named":true,"fields":{"function":{"multiple":true,"required":false,"types":[{"type":"identifier","named":true}]},"receiver":{"multiple":false,"required":false,"types":[{"type":"expression","named":true}]}}},
 {"type":"identifier","named":true,"fields":{}},
 {"type":"number","named":true},
 {"type":"null","named":true,"extra":true},
 {"type":"null","named":false},
 {"type":"this","named":true},
 {"type":";","named":false}
]`

var schemaForward = []string{
	"FIELD_REMOVED REMOVAL call:true arguments -",
	"FIELD_ADDED ADDITION call:true receiver -",
	"FIELD_REQUIRED_CHANGED CARDINALITY_WIDENED call:true function -",
	"FIELD_MULTIPLE_CHANGED CARDINALITY_WIDENED call:true function -",
	"NODE_REMOVED REMOVAL comment:true - -",
	"SUBTYPE_REMOVED REMOVAL expression:true - call:true",
	"SUBTYPE_ADDED ADDITION expression:true - number:true",
	"FIELDS_PRESENCE_CHANGED IDENTITY identifier:true - -",
	"EXTRA_CHANGED CLASSIFICATION null:true - -",
	"NODE_ADDED ADDITION number:true - -",
	"ROOT_CHANGED IDENTITY program:true - -",
	"CHILDREN_REQUIRED_CHANGED CARDINALITY_NARROWED program:true - -",
	"CHILDREN_MULTIPLE_CHANGED CARDINALITY_NARROWED program:true - -",
	"CHILDREN_TYPE_ADDED ADDITION program:true - expression:true",
	"FIELD_TYPE_REMOVED REMOVAL statement:true value ;:false",
	"FIELD_TYPE_ADDED ADDITION statement:true value number:true",
	"CHILDREN_PRESENCE_CHANGED ADDITION statement:true - -",
	"CHILDREN_TYPE_ADDED ADDITION statement:true - identifier:true",
	"NODE_NAMED_CHANGED IDENTITY this:false - -",
}

var schemaReverse = []string{
	"FIELD_REMOVED REMOVAL call:true receiver -",
	"FIELD_ADDED ADDITION call:true arguments -",
	"FIELD_REQUIRED_CHANGED CARDINALITY_NARROWED call:true function -",
	"FIELD_MULTIPLE_CHANGED CARDINALITY_NARROWED call:true function -",
	"NODE_ADDED ADDITION comment:true - -",
	"SUBTYPE_REMOVED REMOVAL expression:true - number:true",
	"SUBTYPE_ADDED ADDITION expression:true - call:true",
	"FIELDS_PRESENCE_CHANGED IDENTITY identifier:true - -",
	"EXTRA_CHANGED CLASSIFICATION null:true - -",
	"NODE_REMOVED REMOVAL number:true - -",
	"ROOT_CHANGED IDENTITY program:true - -",
	"CHILDREN_REQUIRED_CHANGED CARDINALITY_WIDENED program:true - -",
	"CHILDREN_MULTIPLE_CHANGED CARDINALITY_WIDENED program:true - -",
	"CHILDREN_TYPE_REMOVED REMOVAL program:true - expression:true",
	"FIELD_TYPE_REMOVED REMOVAL statement:true value number:true",
	"FIELD_TYPE_ADDED ADDITION statement:true value ;:false",
	"CHILDREN_PRESENCE_CHANGED REMOVAL statement:true - -",
	"CHILDREN_TYPE_REMOVED REMOVAL statement:true - identifier:true",
	"NODE_NAMED_CHANGED IDENTITY this:true - -",
}

func acceptedNodes(t *testing.T, doc string) []Node {
	t.Helper()
	got := Check([]byte(doc))
	if got.Assessment != "PASS" {
		t.Fatalf("fixture invalid: %+v", got)
	}
	return got.Nodes
}

func changeLines(changes []Change) []string {
	lines := make([]string, 0, len(changes))
	for _, c := range changes {
		field, member := c.Field, "-"
		if field == "" {
			field = "-"
		}
		if c.Member != nil {
			member = fmt.Sprintf("%s:%t", c.Member.Type, c.Member.Named)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s:%t %s %s", c.Code, c.Risk, c.Node.Type, c.Node.Named, field, member))
	}
	return lines
}

func TestDirectionalDiff(t *testing.T) {
	before, after := acceptedNodes(t, schemaBase), acceptedNodes(t, schemaCandidate)
	for _, tc := range []struct {
		name string
		a, b []Node
		want []string
	}{
		{"forward", before, after, schemaForward}, {"reverse", after, before, schemaReverse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.a, tc.b)
			if lines := changeLines(got); !reflect.DeepEqual(lines, tc.want) {
				t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
	forward := Diff(before, after)
	byCode := map[string]Change{}
	for _, c := range forward {
		byCode[c.Code+" "+c.Node.Type+" "+c.Field] = c
	}
	for _, tc := range []struct{ key, before, after string }{
		{"FIELD_REQUIRED_CHANGED call function", "true", "false"},
		{"ROOT_CHANGED program ", "true", "null"},
		{"FIELDS_PRESENCE_CHANGED identifier ", "null", "{}"},
		{"NODE_REMOVED comment ", `{"extra":true,"named":true,"type":"comment"}`, "null"},
		{"NODE_NAMED_CHANGED this ", `{"named":false,"type":"this"}`, `{"named":true,"type":"this"}`},
		{"FIELD_TYPE_REMOVED statement value", `{"type":";","named":false}`, "null"},
	} {
		c, exists := byCode[tc.key]
		if !exists {
			t.Fatalf("missing %s", tc.key)
		}
		a, _ := json.Marshal(c.Before)
		b, _ := json.Marshal(c.After)
		if string(a) != tc.before || string(b) != tc.after {
			t.Errorf("%s: %s -> %s", tc.key, a, b)
		}
	}
	if got := Diff(before, before); len(got) != 0 {
		t.Fatalf("self diff: %+v", got)
	}
}

func TestSubtypePresenceDiff(t *testing.T) {
	before := acceptedNodes(t, `[{"type":"a","named":true},{"type":"s","named":true}]`)
	after := acceptedNodes(t, `[{"type":"a","named":true},{"type":"s","named":true,"subtypes":[{"type":"a","named":true}]}]`)
	want := []string{"SUBTYPES_PRESENCE_CHANGED ADDITION s:true - -", "SUBTYPE_ADDED ADDITION s:true - a:true"}
	if got := changeLines(Diff(before, after)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	want = []string{"SUBTYPES_PRESENCE_CHANGED REMOVAL s:true - -", "SUBTYPE_REMOVED REMOVAL s:true - a:true"}
	if got := changeLines(Diff(after, before)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPresenceAndNamedIdentity(t *testing.T) {
	before := acceptedNodes(t, `[{"type":"a","named":true},{"type":"a","named":false}]`)
	after := acceptedNodes(t, `[{"type":"a","named":true}]`)
	want := []string{"NODE_REMOVED REMOVAL a:false - -"}
	if got := changeLines(Diff(before, after)); !reflect.DeepEqual(got, want) {
		t.Fatalf("retained spelling is not a named change: %v", got)
	}
	falseFlag := acceptedNodes(t, `[{"type":"a","named":true,"root":false,"extra":false}]`)
	if got := changeLines(Diff(after, falseFlag)); !reflect.DeepEqual(got, []string{"ROOT_CHANGED IDENTITY a:true - -", "EXTRA_CHANGED CLASSIFICATION a:true - -"}) {
		t.Fatalf("absent vs false: %v", got)
	}
}

func TestDiffOrderIndependent(t *testing.T) {
	reorder := func(doc string) string {
		var values []map[string]any
		if err := json.Unmarshal([]byte(doc), &values); err != nil {
			t.Fatal(err)
		}
		var reverse func(any)
		reverse = func(v any) {
			switch x := v.(type) {
			case []any:
				for i, j := 0, len(x)-1; i < j; i, j = i+1, j-1 {
					x[i], x[j] = x[j], x[i]
				}
				for _, child := range x {
					reverse(child)
				}
			case map[string]any:
				for _, child := range x {
					reverse(child)
				}
			}
		}
		for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
			values[i], values[j] = values[j], values[i]
		}
		for _, v := range values {
			reverse(v)
		}
		data, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	a, b := acceptedNodes(t, schemaBase), acceptedNodes(t, schemaCandidate)
	shuffledA, shuffledB := acceptedNodes(t, reorder(schemaBase)), acceptedNodes(t, reorder(schemaCandidate))
	if got := Diff(a, shuffledA); len(got) != 0 {
		t.Fatalf("reorder diff %+v", got)
	}
	want, _ := json.Marshal(Diff(a, b))
	got, _ := json.Marshal(Diff(shuffledA, shuffledB))
	if string(got) != string(want) {
		t.Fatalf("reordered findings:\n%s\n%s", got, want)
	}
}
