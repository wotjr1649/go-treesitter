package main

// Field-map summary of two generated parser.c files, for CLI-versus-CLI rehearsals: tree-sitter generate writes each rule's
// fields into ts_field_map_slices (production id -> entry range) and ts_field_map_entries ({field, child index, inherited}).
// A CLI that changes how fields are inherited (tree-sitter PR 6050) changes these tables, which the node-types.json diff cannot show.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var fieldEntryRx = regexp.MustCompile(`^\s*\{(field_\w+), (\d+)(, \.inherited = true)?\}`)

type rehearsalFieldMap struct {
	Assessment string            `json:"assessment"` // identical, differs
	Slices     [2]int            `json:"slices"`     // pinned, candidate: productions with fields
	Entries    [2]int            `json:"entries"`
	Inherited  [2]int            `json:"inherited"`
	Fields     map[string][2]int `json:"changedFields,omitempty"` // entries per field name (name+" (inherited)"), only where the counts differ
}

// fieldMapOf returns the normalized slice lines and entry lines of parser.c; a grammar without fields has neither array.
func fieldMapOf(c []byte) (slices, entries []string) {
	block := func(start string) []string {
		s := string(c)
		i := strings.Index(s, start)
		if i < 0 {
			return nil
		}
		s = s[i:]
		if j := strings.Index(s, "\n};"); j >= 0 {
			s = s[:j]
		}
		return strings.Split(s, "\n")[1:]
	}
	for _, l := range block("ts_field_map_slices[") {
		if l = strings.TrimSpace(l); l != "" {
			slices = append(slices, l)
		}
	}
	for _, l := range block("ts_field_map_entries[]") {
		if m := fieldEntryRx.FindStringSubmatch(l); m != nil { // "[n] =" index lines carry no information beyond the order
			entries = append(entries, m[1]+","+m[2]+map[bool]string{true: ",inherited", false: ""}[m[3] != ""])
		}
	}
	return
}

func fieldMapSummary(pinned, candidate []byte) *rehearsalFieldMap {
	ps, pe := fieldMapOf(pinned)
	cs, ce := fieldMapOf(candidate)
	f := &rehearsalFieldMap{Assessment: "identical", Slices: [2]int{len(ps), len(cs)}, Entries: [2]int{len(pe), len(ce)}}
	if strings.Join(ps, "\n") != strings.Join(cs, "\n") || strings.Join(pe, "\n") != strings.Join(ce, "\n") {
		f.Assessment = "differs"
	}
	counts := map[string]*[2]int{}
	for side, entries := range [][]string{pe, ce} {
		for _, e := range entries {
			name, _, _ := strings.Cut(e, ",")
			if strings.HasSuffix(e, ",inherited") {
				name += " (inherited)"
				f.Inherited[side]++
			}
			if counts[name] == nil {
				counts[name] = new([2]int)
			}
			counts[name][side]++
		}
	}
	for name, n := range counts {
		if n[0] != n[1] {
			if f.Fields == nil {
				f.Fields = map[string][2]int{}
			}
			f.Fields[name] = *n
		}
	}
	return f
}

// text is the table cell: "identical", or what moved.
func (f *rehearsalFieldMap) text() string {
	if f == nil {
		return "-"
	}
	if f.Assessment == "identical" {
		return "identical"
	}
	names := make([]string, 0, len(f.Fields))
	for n := range f.Fields {
		names = append(names, n)
	}
	sort.Strings(names)
	return fmt.Sprintf("differs: entries %d→%d, inherited %d→%d, %d field names with other counts", f.Entries[0], f.Entries[1], f.Inherited[0], f.Inherited[1], len(names))
}
