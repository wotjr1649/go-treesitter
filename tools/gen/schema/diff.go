package schema

import (
	"reflect"
	"sort"
)

// Change describes a directional static schema change, not a parser or SemVer verdict.
type Change struct {
	Code   string `json:"code"`
	Risk   string `json:"risk"`
	Node   Ref    `json:"node"`
	Field  string `json:"field,omitempty"`
	Member *Ref   `json:"member,omitempty"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Code names, risk categories and order follow tree-sitter-grammar-kit 0a12156 (MIT).
// The implementation is independent; the comparison never expands supertypes.
var changeCodes = []string{
	"NODE_REMOVED", "NODE_ADDED", "NODE_NAMED_CHANGED", "ROOT_CHANGED", "EXTRA_CHANGED",
	"FIELDS_PRESENCE_CHANGED", "FIELD_REMOVED", "FIELD_ADDED", "FIELD_REQUIRED_CHANGED", "FIELD_MULTIPLE_CHANGED", "FIELD_TYPE_REMOVED", "FIELD_TYPE_ADDED",
	"CHILDREN_PRESENCE_CHANGED", "CHILDREN_REQUIRED_CHANGED", "CHILDREN_MULTIPLE_CHANGED", "CHILDREN_TYPE_REMOVED", "CHILDREN_TYPE_ADDED",
	"SUBTYPES_PRESENCE_CHANGED", "SUBTYPE_REMOVED", "SUBTYPE_ADDED",
}

// Diff compares schemas accepted by Check. It preserves absent versus explicit false/empty.
func Diff(before, after []Node) []Change {
	old, next := make(map[Ref]Node), make(map[Ref]Node)
	removed, added := make(map[string][]Node), make(map[string][]Node)
	for _, n := range before {
		old[n.Ref] = n
	}
	for _, n := range after {
		next[n.Ref] = n
	}
	for r, n := range old {
		if _, ok := next[r]; !ok {
			removed[r.Type] = append(removed[r.Type], n)
		}
	}
	for r, n := range next {
		if _, ok := old[r]; !ok {
			added[r.Type] = append(added[r.Type], n)
		}
	}
	changes := make([]Change, 0)
	emit := func(code, risk string, node Ref, field string, member *Ref, a, b any) {
		changes = append(changes, Change{code, risk, node, field, member, a, b})
	}
	for spelling, nodes := range removed {
		candidates := added[spelling]
		if len(nodes) == 1 && len(candidates) == 1 {
			emit("NODE_NAMED_CHANGED", "IDENTITY", nodes[0].Ref, "", nil, nodeValue(nodes[0]), nodeValue(candidates[0]))
			delete(added, spelling)
		} else {
			for _, n := range nodes {
				emit("NODE_REMOVED", "REMOVAL", n.Ref, "", nil, nodeValue(n), nil)
			}
		}
	}
	for _, nodes := range added {
		for _, n := range nodes {
			emit("NODE_ADDED", "ADDITION", n.Ref, "", nil, nil, nodeValue(n))
		}
	}
	for ref, a := range old {
		b, exists := next[ref]
		if !exists {
			continue
		}
		if !reflect.DeepEqual(a.Root, b.Root) {
			emit("ROOT_CHANGED", "IDENTITY", ref, "", nil, a.Root, b.Root)
		}
		if !reflect.DeepEqual(a.Extra, b.Extra) {
			emit("EXTRA_CHANGED", "CLASSIFICATION", ref, "", nil, a.Extra, b.Extra)
		}
		if (a.Fields == nil) != (b.Fields == nil) {
			emit("FIELDS_PRESENCE_CHANGED", "IDENTITY", ref, "", nil, fieldsValue(a.Fields), fieldsValue(b.Fields))
		}
		for name, field := range a.Fields {
			other, found := b.Fields[name]
			if !found {
				emit("FIELD_REMOVED", "REMOVAL", ref, name, nil, setValue(&field), nil)
			} else {
				changes = append(changes, compareSet("FIELD", ref, name, &field, &other)...)
			}
		}
		for name, field := range b.Fields {
			if _, found := a.Fields[name]; !found {
				emit("FIELD_ADDED", "ADDITION", ref, name, nil, nil, setValue(&field))
			}
		}
		if (a.Children == nil) != (b.Children == nil) {
			risk := "ADDITION"
			if b.Children == nil {
				risk = "REMOVAL"
			}
			emit("CHILDREN_PRESENCE_CHANGED", risk, ref, "", nil, setValue(a.Children), setValue(b.Children))
		}
		changes = append(changes, compareSet("CHILDREN", ref, "", a.Children, b.Children)...)
		if (a.Subtypes == nil) != (b.Subtypes == nil) {
			risk := "ADDITION"
			if b.Subtypes == nil {
				risk = "REMOVAL"
			}
			emit("SUBTYPES_PRESENCE_CHANGED", risk, ref, "", nil, canonicalRefs(a.Subtypes), canonicalRefs(b.Subtypes))
		}
		changes = append(changes, compareAlternatives("SUBTYPE", ref, "", a.Subtypes, b.Subtypes)...)
	}
	ranks := make(map[string]int, len(changeCodes))
	for i, code := range changeCodes {
		ranks[code] = i
	}
	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if a.Node != b.Node {
			return refLess(a.Node, b.Node)
		}
		if a.Code != b.Code {
			return ranks[a.Code] < ranks[b.Code]
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Member == nil || b.Member == nil {
			return a.Member == nil && b.Member != nil
		}
		return refLess(*a.Member, *b.Member)
	})
	return changes
}

func compareSet(prefix string, node Ref, field string, a, b *Set) []Change {
	out := make([]Change, 0)
	var before, after []Ref
	if a != nil {
		before = a.Types
	}
	if b != nil {
		after = b.Types
	}
	if a != nil && b != nil {
		if a.Required != b.Required {
			out = append(out, Change{prefix + "_REQUIRED_CHANGED", cardinality(b.Required), node, field, nil, a.Required, b.Required})
		}
		if a.Multiple != b.Multiple {
			out = append(out, Change{prefix + "_MULTIPLE_CHANGED", cardinality(!b.Multiple), node, field, nil, a.Multiple, b.Multiple})
		}
	}
	return append(out, compareAlternatives(prefix+"_TYPE", node, field, before, after)...)
}

func compareAlternatives(prefix string, node Ref, field string, before, after []Ref) []Change {
	old, next := make(map[Ref]bool), make(map[Ref]bool)
	for _, r := range before {
		old[r] = true
	}
	for _, r := range after {
		next[r] = true
	}
	out := make([]Change, 0)
	for r := range old {
		if !next[r] {
			member := r
			out = append(out, Change{prefix + "_REMOVED", "REMOVAL", node, field, &member, member, nil})
		}
	}
	for r := range next {
		if !old[r] {
			member := r
			out = append(out, Change{prefix + "_ADDED", "ADDITION", node, field, &member, nil, member})
		}
	}
	return out
}

func cardinality(narrower bool) string {
	if narrower {
		return "CARDINALITY_NARROWED"
	}
	return "CARDINALITY_WIDENED"
}

func refLess(a, b Ref) bool {
	if a.Type != b.Type {
		return a.Type < b.Type
	}
	return !a.Named && b.Named
}

func canonicalRefs(refs []Ref) []Ref {
	if refs == nil {
		return nil
	}
	out := append([]Ref{}, refs...)
	sort.Slice(out, func(i, j int) bool { return refLess(out[i], out[j]) })
	return out
}

func setValue(s *Set) any {
	if s == nil {
		return nil
	}
	return map[string]any{"multiple": s.Multiple, "required": s.Required, "types": canonicalRefs(s.Types)}
}

func fieldsValue(fields map[string]Set) any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for k, s := range fields {
		out[k] = setValue(&s)
	}
	return out
}

func nodeValue(n Node) any {
	out := map[string]any{"type": n.Type, "named": n.Named}
	if n.Root != nil {
		out["root"] = *n.Root
	}
	if n.Extra != nil {
		out["extra"] = *n.Extra
	}
	if n.Fields != nil {
		out["fields"] = fieldsValue(n.Fields)
	}
	if n.Children != nil {
		out["children"] = setValue(n.Children)
	}
	if n.Subtypes != nil {
		out["subtypes"] = canonicalRefs(n.Subtypes)
	}
	return out
}
