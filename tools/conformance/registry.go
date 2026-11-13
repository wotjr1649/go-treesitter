package conformance

import (
	"sort"

	treesitter "github.com/wotjr1649/go-treesitter"
)

var languages = map[string]func() *treesitter.Language{}

// register is called from the lang_<name>.go files (build tag !no_<name> leaves a grammar out of the binary).
func register(name string, fn func() *treesitter.Language) { languages[name] = fn }

// Linked lists the corpus language names linked into this binary.
func Linked() []string {
	var out []string
	for k := range languages {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
