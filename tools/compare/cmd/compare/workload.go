package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

// The comparison languages and their names in the three data trees they come from.
type langInfo struct {
	name     string // compare name; also the worker -lang value and the ciconf data directory
	large    string // tools/bench/ci/large.json key and benchci fetch directory
	product  string // tools/gen/pins.json key
	nativeFn string // the grammar's entry function
}

var allLangs = []langInfo{
	{"c", "c", "c", "tree_sitter_c"},
	{"go", "golang", "golang", "tree_sitter_go"},
	{"javascript", "javascript", "javascript", "tree_sitter_javascript"},
	{"json", "json", "json", "tree_sitter_json"},
	{"python", "python", "python", "tree_sitter_python"},
}

const (
	smallMaxBytes = 25 * 1024
	smallClean    = 18
	smallErrors   = 6
)

// EditSpec is one frozen edit of an input. NewPath holds the edited text next to the input.
type EditSpec struct {
	Kind      string `json:"kind"` // token | newline | insert | delete | replace
	Start     uint32 `json:"start"`
	OldEnd    uint32 `json:"old_end"`
	NewEnd    uint32 `json:"new_end"`
	NewPath   string `json:"new_path"`
	NewSHA256 string `json:"new_sha256"`
	SamePts   bool   `json:"equal_points"` // old and new end points coincide (the timing workload requires it)
}

// Input is one frozen input. Paths are relative to the workload directory.
type Input struct {
	ID     string     `json:"id"`
	Lang   string     `json:"lang"`
	Group  string     `json:"group"` // small | large
	Path   string     `json:"path"`
	Bytes  int        `json:"bytes"`
	SHA256 string     `json:"sha256"`
	Source string     `json:"source"`          // corpus file, or the large manifest key
	Clean  *bool      `json:"clean,omitempty"` // product native reference accepts it without error nodes (small only)
	Token  *EditSpec  `json:"token_edit,omitempty"`
	Edits  []EditSpec `json:"edits,omitempty"` // correctness edits: ciconf insert/delete/replace, plus the newline case
}

// Workload is workload.json: every input and edit of the run, frozen before any candidate output exists.
type Workload struct {
	Schema     int      `json:"schema"`
	Seed       string   `json:"seed"`
	Inputs     []Input  `json:"inputs"`
	Shortages  []string `json:"shortages,omitempty"`
	Exclusions []string `json:"exclusions,omitempty"`
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// tokenEdit replaces the last byte of the first identifier-like ASCII token of at least five bytes found from the
// middle of src onward (wrapping once). The result has the same length and stays on one line, so the old and new end
// points are equal. It never looks at a candidate's behavior.
func tokenEdit(src []byte) (start, end uint32, nw []byte, ok bool) {
	n := len(src)
	for k := 0; k < n; k++ {
		i := (n/2 + k) % n
		if !isIdent(src[i]) || i > 0 && isIdent(src[i-1]) || src[i] >= '0' && src[i] <= '9' {
			continue
		}
		j := i
		for j < n && isIdent(src[j]) {
			j++
		}
		if j-i >= 5 {
			nw = bytes.Clone(src)
			if nw[j-1] == 'x' {
				nw[j-1] = 'y'
			} else {
				nw[j-1] = 'x'
			}
			return uint32(i), uint32(j), nw, true
		}
	}
	return 0, 0, nil, false
}

// newlineEdit turns one ASCII space inside a line, found from the middle of src onward, into a newline: one byte for
// one byte, but the end points differ, which exposes a binding that maps the old end point into the new one.
func newlineEdit(src []byte) (start uint32, nw []byte, ok bool) {
	n := len(src)
	for k := 1; k < n-1; k++ {
		i := (n/2 + k) % n
		if i < 1 || i >= n-1 || src[i] != ' ' {
			continue
		}
		a, b := src[i-1], src[i+1]
		if a > ' ' && a < 0x7f && b > ' ' && b < 0x7f {
			nw = bytes.Clone(src)
			nw[i] = '\n'
			return uint32(i), nw, true
		}
	}
	return 0, nil, false
}

type selectOpts struct {
	data, large, manifest, out string
	langs                      []string
	seed                       string
}

func cmdSelect(args []string) error {
	fs := flag.NewFlagSet("select", flag.ExitOnError)
	var o selectOpts
	var langs string
	fs.StringVar(&o.data, "data", "", "ciconf data directory (corpus/, native/dumps/, edits/)")
	fs.StringVar(&o.large, "large", "", "benchci fetch output directory (<lang>/<file>)")
	fs.StringVar(&o.manifest, "large-manifest", "", "tools/bench/ci/large.json")
	fs.StringVar(&o.out, "out", "", "workload directory to create")
	fs.StringVar(&langs, "langs", "c,go,javascript,json,python", "comma-separated languages")
	fs.StringVar(&o.seed, "seed", "s10-compare-1", "manifest seed (orders the AB/BA rounds)")
	fs.Parse(args)
	o.langs = strings.Split(langs, ",")
	if o.data == "" || o.out == "" {
		return errors.New("select needs -data and -out")
	}
	w, err := selectWorkload(o)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(o.out, "workload.json"), w)
}

func langByName(n string) (langInfo, bool) {
	for _, l := range allLangs {
		if l.name == n {
			return l, true
		}
	}
	return langInfo{}, false
}

func readCSV(p string) ([][]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	return rows[1:], nil
}

// dumpClean reads the root_has_error flag from a native dump header: "#tsg-dump/1 root_has_error=0 nodes=N".
func dumpClean(dump []byte) (bool, error) {
	line, _, _ := bytes.Cut(dump, []byte("\n"))
	switch {
	case bytes.Contains(line, []byte("root_has_error=0")):
		return true, nil
	case bytes.Contains(line, []byte("root_has_error=1")):
		return false, nil
	}
	return false, fmt.Errorf("unrecognized dump header %q", line)
}

func selectWorkload(o selectOpts) (*Workload, error) {
	w := &Workload{Schema: harness.Schema, Seed: o.seed}
	corpus, err := readCSV(filepath.Join(o.data, "corpus", "manifest.csv"))
	if err != nil {
		return nil, err
	}
	editRows, err := readCSV(filepath.Join(o.data, "edits", "manifest.csv"))
	if err != nil {
		return nil, err
	}
	for _, name := range o.langs {
		li, ok := langByName(name)
		if !ok {
			return nil, fmt.Errorf("unknown language %q", name)
		}
		type cand struct {
			id, sum, file string
			src           []byte
			clean         bool
		}
		var cs []cand
		seen := map[string]bool{}
		for _, r := range corpus { // id, lang, source_ref, bytes, sha256, corpus_file, test_name, attrs
			if r[1] != name {
				continue
			}
			base := filepath.Base(r[0])
			src, err := os.ReadFile(filepath.Join(o.data, "corpus", name, base+".in"))
			if err != nil {
				return nil, err
			}
			switch {
			case len(src) == 0:
				w.Exclusions = append(w.Exclusions, r[0]+": empty")
			case len(src) > smallMaxBytes:
				w.Exclusions = append(w.Exclusions, r[0]+": over 25 KiB")
			case !utf8.Valid(src):
				w.Exclusions = append(w.Exclusions, r[0]+": not valid UTF-8")
			case sha(src) != r[4]:
				return nil, fmt.Errorf("%s: sha256 differs from the corpus manifest", r[0])
			case seen[r[4]]:
				w.Exclusions = append(w.Exclusions, r[0]+": duplicate bytes")
			default:
				seen[r[4]] = true
				dump, err := os.ReadFile(filepath.Join(o.data, "native", "dumps", name, base+".dump"))
				if err != nil {
					return nil, fmt.Errorf("%s: no product native reference: %w", r[0], err)
				}
				clean, err := dumpClean(dump)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", r[0], err)
				}
				cs = append(cs, cand{r[0], r[4], r[5], src, clean})
			}
		}
		slices.SortFunc(cs, func(a, b cand) int {
			if c := strings.Compare(a.sum, b.sum); c != 0 {
				return c
			}
			return strings.Compare(a.id, b.id)
		})
		var picked []cand
		nc, ne := 0, 0
		for _, c := range cs {
			if c.clean && nc < smallClean {
				nc++
				picked = append(picked, c)
			} else if !c.clean && ne < smallErrors {
				ne++
				picked = append(picked, c)
			}
		}
		if nc < smallClean || ne < smallErrors {
			w.Shortages = append(w.Shortages, fmt.Sprintf("%s: %d clean of %d, %d with errors of %d", name, nc, smallClean, ne, smallErrors))
		}
		nlDone := false
		for _, c := range picked {
			in := Input{ID: c.id, Lang: name, Group: "small", Bytes: len(c.src), SHA256: c.sum, Source: c.file}
			cl := c.clean
			in.Clean = &cl
			in.Path = filepath.ToSlash(filepath.Join("inputs", name, filepath.Base(c.id)+".src"))
			if err := writeFileAll(filepath.Join(o.out, in.Path), c.src); err != nil {
				return nil, err
			}
			if err := addTokenEdit(&in, c.src, o.out); err != nil {
				return nil, err
			}
			if !nlDone && c.clean {
				if st, nw, ok := newlineEdit(c.src); ok {
					if err := addEdit(&in, "newline", c.src, nw, st, st+1, st+1, o.out); err != nil {
						return nil, err
					}
					nlDone = true
				}
			}
			for _, r := range editRows { // id, lang, base_ref, start, old_end, new_end, old_sha256, new_sha256, kind, seed
				if r[2] != c.id {
					continue
				}
				old, err := os.ReadFile(filepath.Join(o.data, "edits", name, filepath.Base(r[0])+".old"))
				if err != nil {
					return nil, err
				}
				nw, err := os.ReadFile(filepath.Join(o.data, "edits", name, filepath.Base(r[0])+".new"))
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(old, c.src) || sha(nw) != r[7] {
					return nil, fmt.Errorf("%s: edit files do not match the manifest", r[0])
				}
				var v [3]uint64
				for i := range v {
					if v[i], err = strconv.ParseUint(r[3+i], 10, 32); err != nil {
						return nil, err
					}
				}
				if err := addEdit(&in, r[8], c.src, nw, uint32(v[0]), uint32(v[1]), uint32(v[2]), o.out); err != nil {
					return nil, err
				}
			}
			w.Inputs = append(w.Inputs, in)
		}
		if !nlDone {
			w.Shortages = append(w.Shortages, name+": no newline-changing edit found")
		}
		// Large inputs: the pinned manifest entries of this language, verified by benchci fetch before this runs.
		if o.large != "" {
			entries, err := largeEntries(o.manifest, li.large)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				src, err := os.ReadFile(filepath.Join(o.large, li.large, e.File))
				if err != nil {
					return nil, err
				}
				if sha(src) != e.SHA256 {
					return nil, fmt.Errorf("large %s/%s: sha256 %s, pinned %s", li.large, e.File, sha(src), e.SHA256)
				}
				in := Input{ID: name + "/large/" + e.File, Lang: name, Group: "large", Bytes: len(src), SHA256: sha(src), Source: "tools/bench/ci/large.json:" + li.large + "/" + e.File}
				in.Path = filepath.ToSlash(filepath.Join("inputs", name, "large-"+e.File+".src"))
				if err := writeFileAll(filepath.Join(o.out, in.Path), src); err != nil {
					return nil, err
				}
				if err := addTokenEdit(&in, src, o.out); err != nil {
					return nil, err
				}
				w.Inputs = append(w.Inputs, in)
			}
		}
	}
	return w, nil
}

type largeEntry struct{ Lang, File, SHA256 string }

func largeEntries(manifest, lang string) ([]largeEntry, error) {
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return nil, err
	}
	var all []struct {
		Lang   string `json:"lang"`
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	var out []largeEntry
	for _, e := range all {
		if e.Lang == lang {
			out = append(out, largeEntry{e.Lang, e.File, e.SHA256})
		}
	}
	return out, nil
}

func addTokenEdit(in *Input, src []byte, out string) error {
	st, end, nw, ok := tokenEdit(src)
	if !ok {
		return nil // no applicable token: the input keeps its full-parse rows and an unavailable incremental cell
	}
	if err := addEdit(in, "token", src, nw, st, end, end, out); err != nil {
		return err
	}
	t := in.Edits[len(in.Edits)-1] // the timing edit is also one of the correctness edits
	in.Token = &t
	return nil
}

// addEdit validates the triple (consistent bytes, points) and stores the edited text next to the input.
func addEdit(in *Input, kind string, old, nw []byte, start, oldEnd, newEnd uint32, out string) error {
	e, err := harness.MakeEdit(old, nw, start, oldEnd, newEnd)
	if err != nil {
		return fmt.Errorf("%s %s edit: %w", in.ID, kind, err)
	}
	if kind == "token" && (e.OldEndRow != e.NewEndRow || e.OldEndCol != e.NewEndCol || oldEnd != newEnd) {
		return fmt.Errorf("%s: token edit does not keep the end point", in.ID)
	}
	sp := EditSpec{Kind: kind, Start: start, OldEnd: oldEnd, NewEnd: newEnd, NewSHA256: sha(nw),
		SamePts: e.OldEndRow == e.NewEndRow && e.OldEndCol == e.NewEndCol}
	sp.NewPath = strings.TrimSuffix(in.Path, ".src") + "." + kind + strconv.Itoa(len(in.Edits)) + ".new"
	if err := writeFileAll(filepath.Join(out, sp.NewPath), nw); err != nil {
		return err
	}
	in.Edits = append(in.Edits, sp)
	return nil
}

func writeFileAll(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAll(p, append(b, '\n'))
}
