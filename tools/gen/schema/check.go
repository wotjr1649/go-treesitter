// Package schema checks and compares the static contract in node-types.json.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Ref struct {
	Type  string `json:"type"`
	Named bool   `json:"named"`
}

type Set struct {
	Multiple bool
	Required bool
	Types    []Ref
}

type Node struct {
	Ref
	Root     *bool
	Extra    *bool
	Fields   map[string]Set
	Children *Set
	Subtypes []Ref
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

type Result struct {
	Assessment string    `json:"assessment"`
	Findings   []Finding `json:"findings"`
	Nodes      []Node    `json:"-"`
}

type inputError struct{ code, path, message string }

func (e *inputError) Error() string { return e.message }

// Check preserves absence separately from false and from an empty fields object.
// Only PASS results expose nodes; callers cannot accidentally diff partial input.
func Check(data []byte) Result {
	c := checker{result: Result{Assessment: "PASS", Findings: []Finding{}}, index: map[Ref]int{}}
	if !utf8.Valid(data) {
		c.add("JSON_INVALID_UTF8", "error", "", "input is not valid UTF-8")
		return c.finish()
	}
	if offset, ok := validSurrogates(data); !ok {
		c.add("JSON_INVALID_UNICODE", "error", "", fmt.Sprintf("unpaired surrogate escape at byte %d", offset))
		return c.finish()
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	err := c.readDocument(d)
	if err == nil {
		_, tail := d.Token()
		if tail != io.EOF {
			err = &inputError{"JSON_TRAILING_VALUE", "", "expected end of document"}
		}
	}
	if err != nil {
		code, path := "JSON_INVALID", ""
		var ie *inputError
		if errors.As(err, &ie) {
			code, path = ie.code, ie.path
		} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "unexpected EOF") || strings.Contains(err.Error(), "unexpected end of JSON input") {
			code = "JSON_TRUNCATED"
		}
		c.result = Result{Assessment: "PASS", Findings: []Finding{}}
		c.totalFindings = 0
		c.add(code, "error", path, err.Error())
		return c.finish()
	}
	nodes, index := c.nodes, c.index
	for _, use := range c.references {
		if _, exists := index[use.ref]; !exists {
			code, severity := "REFERENCE_UNDECLARED", "warning"
			if use.subtype {
				code, severity = "REFERENCE_UNRESOLVED", "error"
			}
			c.add(code, severity, use.path, fmt.Sprintf("undeclared type %q (named=%t)", use.ref.Type, use.ref.Named))
		}
	}
	graph := map[Ref][]Ref{}
	for _, n := range nodes {
		graph[n.Ref] = n.Subtypes
	}
	colors := map[Ref]uint8{}
	type frame struct {
		ref  Ref
		next int
	}
	for _, n := range nodes {
		if colors[n.Ref] != 0 {
			continue
		}
		stack := []frame{{ref: n.Ref}}
		colors[n.Ref] = 1
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			children := graph[top.ref]
			if top.next == len(children) {
				colors[top.ref] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			child := children[top.next]
			edge := top.next
			top.next++
			if colors[child] == 1 {
				c.add("SUPERTYPE_CYCLE", "error", fmt.Sprintf("/%d/subtypes/%d", index[top.ref], edge), "subtype edge closes a cycle")
			} else if colors[child] == 0 {
				colors[child] = 1
				stack = append(stack, frame{ref: child})
			}
		}
	}
	if c.result.Assessment == "PASS" {
		c.result.Nodes = nodes
	}
	return c.finish()
}

// readDocument releases each decoded node before reading the next one.
func (c *checker) readDocument(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('[') {
		value, err := readToken(d, token, "", 0)
		if err == nil {
			c.wrong(value, "", "expected node array")
		}
		return err
	}
	count := 0
	for d.More() {
		entry, err := readValue(d, "/"+strconv.Itoa(count), 1)
		if err != nil {
			return err
		}
		c.node(entry, count)
		count++
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if count == 0 {
		c.add("SCHEMA_EMPTY", "error", "", "schema has no nodes")
	}
	return nil
}

func (c *checker) node(entry any, i int) {
	path := "/" + strconv.Itoa(i)
	obj, ok := c.object(entry, path)
	if !ok {
		return
	}
	c.keys(obj, path, "type", "named", "root", "extra", "fields", "children", "subtypes")
	ref, good := c.ref(obj, path)
	n := Node{Ref: ref}
	for _, key := range []string{"root", "extra"} {
		if v, exists := obj[key]; exists {
			b, valid := c.boolean(v, path+"/"+key)
			if valid {
				if key == "root" {
					n.Root = &b
					if b {
						c.roots++
						if c.roots > 1 {
							c.add("ROOT_MULTIPLE", "error", path+"/root", "more than one root node")
						}
					}
				} else {
					n.Extra = &b
				}
			}
		}
	}
	if v, exists := obj["fields"]; exists {
		if fields, valid := c.object(v, path+"/fields"); valid {
			n.Fields = map[string]Set{}
			for _, field := range keys(fields) {
				fp := path + "/fields/" + pointer(field)
				if field == "" {
					c.add("FIELD_NAME_EMPTY", "error", fp, "field name is empty")
				}
				if s := c.set(fields[field], fp); s != nil {
					n.Fields[field] = *s
				}
			}
		}
	}
	if v, exists := obj["children"]; exists {
		n.Children = c.set(v, path+"/children")
	}
	if v, exists := obj["subtypes"]; exists {
		n.Subtypes = c.refs(v, path+"/subtypes", true)
		if list, ok := v.([]any); ok && len(list) == 0 {
			c.add("SUBTYPES_EMPTY", "error", path+"/subtypes", "subtypes is empty")
		}
		_, hasFields := obj["fields"]
		_, hasChildren := obj["children"]
		if hasFields || hasChildren {
			c.add("SUPERTYPE_SHAPE", "error", path, "supertype cannot have fields or children")
		}
	}
	if good {
		if prev, exists := c.index[ref]; exists {
			c.add("NODE_DUPLICATE", "error", path, fmt.Sprintf("duplicate (type, named), first declared at /%d", prev))
		} else {
			c.index[ref] = i
			c.nodes = append(c.nodes, n)
		}
	}
}

// readValue uses decoded key strings, so escape-equivalent duplicates collide.
func readValue(d *json.Decoder, path string, depth int) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	return readToken(d, token, path, depth)
}

func readToken(d *json.Decoder, token json.Token, path string, depth int) (any, error) {
	if (token == json.Delim('{') || token == json.Delim('[')) && depth >= maxJSONDepth {
		return nil, &inputError{"JSON_DEPTH_LIMIT", path, "JSON nesting exceeds 64 container levels"}
	}
	switch token {
	case json.Delim('{'):
		obj := map[string]any{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, &inputError{"JSON_INVALID", path, "object key is not a string"}
			}
			if len(key) > maxKeyBytes {
				return nil, &inputError{"JSON_KEY_LIMIT", path, "object key exceeds 1024 UTF-8 bytes"}
			}
			kp := path + "/" + pointer(key)
			if _, exists := obj[key]; exists {
				return nil, &inputError{"JSON_DUPLICATE_KEY", kp, "duplicate object key"}
			}
			value, err := readValue(d, kp, depth+1)
			if err != nil {
				return nil, err
			}
			obj[key] = value
		}
		if _, err := d.Token(); err != nil {
			return nil, err
		}
		return obj, nil
	case json.Delim('['):
		list := []any{}
		for d.More() {
			value, err := readValue(d, path+"/"+strconv.Itoa(len(list)), depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		if _, err := d.Token(); err != nil {
			return nil, err
		}
		return list, nil
	default:
		return token, nil
	}
}

func pointer(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type referenceUse struct {
	ref     Ref
	path    string
	subtype bool
}

const (
	maxFindings  = 1000
	maxJSONDepth = 64
	maxKeyBytes  = 1024
)

type checker struct {
	result        Result
	references    []referenceUse
	nodes         []Node
	index         map[Ref]int
	roots         int
	totalFindings int
}

func (c *checker) add(code, severity, path, message string) {
	c.totalFindings++
	if c.totalFindings <= maxFindings {
		c.result.Findings = append(c.result.Findings, Finding{code, severity, path, message})
	} else if c.totalFindings == maxFindings+1 {
		c.result.Findings = append(c.result.Findings, Finding{Code: "FINDINGS_TRUNCATED", Severity: "error"})
	}
	if severity != "error" {
		return
	}
	if code == "SCHEMA_KEY_UNSUPPORTED" {
		if c.result.Assessment == "PASS" {
			c.result.Assessment = "BLOCKED"
		}
	} else {
		c.result.Assessment = "FAIL"
	}
}

func (c *checker) finish() Result {
	if c.totalFindings > maxFindings {
		c.result.Findings[maxFindings].Message = fmt.Sprintf("retained first %d of %d findings", maxFindings, c.totalFindings)
		// A truncated warning-only report cannot expose a complete PASS result.
		if c.result.Assessment == "PASS" {
			c.result.Assessment = "FAIL"
		}
	}
	if c.result.Assessment != "PASS" {
		c.result.Nodes = nil
	}
	return c.result
}
func (c *checker) wrong(v any, path, message string) {
	code := "JSON_TYPE"
	if v == nil {
		code = "JSON_NULL"
	}
	c.add(code, "error", path, message)
}
func (c *checker) object(v any, path string) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		c.wrong(v, path, "expected object")
	}
	return m, ok
}
func (c *checker) keys(obj map[string]any, path string, allowed ...string) {
	for _, key := range keys(obj) {
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			c.add("SCHEMA_KEY_UNSUPPORTED", "error", path+"/"+pointer(key), "unsupported key")
		}
	}
}
func (c *checker) required(obj map[string]any, key, path string) (any, bool) {
	v, exists := obj[key]
	if !exists {
		c.add("JSON_MISSING_FIELD", "error", path+"/"+key, "missing required key")
	}
	return v, exists
}
func (c *checker) boolean(v any, path string) (bool, bool) {
	b, ok := v.(bool)
	if !ok {
		c.wrong(v, path, "expected boolean")
	}
	return b, ok
}
func (c *checker) ref(obj map[string]any, path string) (Ref, bool) {
	r := Ref{}
	valid := true
	if v, exists := c.required(obj, "type", path); exists {
		var ok bool
		r.Type, ok = v.(string)
		if !ok {
			c.wrong(v, path+"/type", "expected string")
			valid = false
		}
	} else {
		valid = false
	}
	if v, exists := c.required(obj, "named", path); exists {
		var ok bool
		r.Named, ok = c.boolean(v, path+"/named")
		valid = valid && ok
	} else {
		valid = false
	}
	return r, valid
}
func (c *checker) refs(v any, path string, subtype bool) []Ref {
	list, ok := v.([]any)
	if !ok {
		c.wrong(v, path, "expected reference array")
		return nil
	}
	out := []Ref{}
	seen := map[Ref]bool{}
	for i, entry := range list {
		rp := path + "/" + strconv.Itoa(i)
		obj, ok := c.object(entry, rp)
		if !ok {
			continue
		}
		c.keys(obj, rp, "type", "named")
		ref, good := c.ref(obj, rp)
		if !good {
			continue
		}
		if seen[ref] {
			c.add("MEMBER_DUPLICATE", "error", rp, "duplicate type reference")
			continue
		}
		seen[ref] = true
		out = append(out, ref)
		c.references = append(c.references, referenceUse{ref, rp, subtype})
	}
	return out
}
func (c *checker) set(v any, path string) *Set {
	obj, ok := c.object(v, path)
	if !ok {
		return nil
	}
	c.keys(obj, path, "multiple", "required", "types")
	s := &Set{}
	if v, exists := c.required(obj, "multiple", path); exists {
		s.Multiple, _ = c.boolean(v, path+"/multiple")
	}
	if v, exists := c.required(obj, "required", path); exists {
		s.Required, _ = c.boolean(v, path+"/required")
	}
	if v, exists := c.required(obj, "types", path); exists {
		s.Types = c.refs(v, path+"/types", false)
		if list, ok := v.([]any); ok && len(list) == 0 {
			c.add("TYPES_EMPTY", "error", path+"/types", "types is empty")
		}
	}
	return s
}

// encoding/json replaces lone surrogate escapes; reject them before token decoding.
func validSurrogates(data []byte) (int, bool) {
	inString := false
	hex4 := func(start int) (uint64, bool) {
		if start+4 > len(data) {
			return 0, false
		}
		v, err := strconv.ParseUint(string(data[start:start+4]), 16, 16)
		return v, err == nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		if i+1 >= len(data) {
			continue
		}
		if data[i+1] != 'u' {
			i++
			continue
		}
		v, ok := hex4(i + 2)
		if !ok {
			continue
		}
		if v >= 0xDC00 && v <= 0xDFFF {
			return i, false
		}
		if v >= 0xD800 && v <= 0xDBFF {
			if i+7 >= len(data) || data[i+6] != '\\' || data[i+7] != 'u' {
				return i, false
			}
			low, valid := hex4(i + 8)
			if !valid || low < 0xDC00 || low > 0xDFFF {
				return i, false
			}
			i += 6
		}
		i += 5
	}
	return 0, true
}
