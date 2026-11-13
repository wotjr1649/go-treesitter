package treesitter

import (
	"sync"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// Symbol is a grammar symbol id (the public id C tree-sitter reports for a node). ErrorSymbol is the ERROR node.
type Symbol uint16

// FieldID is a grammar field id; 0 means "no field".
type FieldID uint16

// ErrorSymbol is the symbol of ERROR nodes (C: ts_builtin_sym_error).
const ErrorSymbol Symbol = 0xFFFF

// Language is an opaque, immutable, goroutine-safe description of one generated grammar module. All name data is
// static Go data; reading it never needs an engine instance.
type Language struct {
	g *engine.Grammar

	once   sync.Once
	named  map[string]Symbol // visible/supertype symbols by name, first id wins (C: ts_language_symbol_for_name)
	anon   map[string]Symbol
	byName map[string][]Symbol // every symbol id carrying the name (named, anonymous, hidden)
	fields map[string]FieldID
}

// symbolsNamed lists every symbol id whose name is kind (empty for an unknown kind).
func (l *Language) symbolsNamed(kind string) []Symbol {
	l.index()
	return l.byName[kind]
}

// NewLanguage wraps a generated grammar. It is for generated grammar modules (the argument type is internal).
func NewLanguage(g *engine.Grammar) *Language { return &Language{g: g} }

func (l *Language) index() {
	l.once.Do(func() {
		m := &l.g.Meta
		l.named, l.anon = map[string]Symbol{}, map[string]Symbol{}
		for i, name := range m.Symbols {
			f := m.SymbolFlags[i]
			if f&(engine.MetaVisible|engine.MetaSupertype) == 0 {
				continue
			}
			set := l.anon
			if f&engine.MetaNamed != 0 {
				set = l.named
			}
			if _, dup := set[name]; !dup {
				set[name] = Symbol(i)
			}
		}
		l.byName = make(map[string][]Symbol, len(m.Symbols)+1)
		for i, name := range m.Symbols {
			l.byName[name] = append(l.byName[name], Symbol(i))
		}
		l.byName["ERROR"] = append(l.byName["ERROR"], ErrorSymbol)
		l.fields = make(map[string]FieldID, len(m.Fields))
		for i := 1; i < len(m.Fields); i++ {
			if _, dup := l.fields[m.Fields[i]]; !dup {
				l.fields[m.Fields[i]] = FieldID(i)
			}
		}
	})
}

// Name is the grammar's name (for example "csharp").
func (l *Language) Name() string { return l.g.Name }

// SymbolCount is the number of symbol ids (without ERROR).
func (l *Language) SymbolCount() int { return len(l.g.Meta.Symbols) }

// FieldCount is the number of field ids including the empty id 0.
func (l *Language) FieldCount() int { return len(l.g.Meta.Fields) }

// SymbolForName finds a visible symbol by name. Named and anonymous symbols may share a name, so the kind is part of
// the key. "ERROR" with named=true is ErrorSymbol. The second result is false when there is no such symbol.
func (l *Language) SymbolForName(name string, named bool) (Symbol, bool) {
	if named && name == "ERROR" {
		return ErrorSymbol, true
	}
	l.index()
	set := l.anon
	if named {
		set = l.named
	}
	s, ok := set[name]
	return s, ok
}

// SymbolName is the name of s ("" when s is not a symbol of this language).
func (l *Language) SymbolName(s Symbol) string {
	if s == ErrorSymbol {
		return "ERROR"
	}
	if int(s) < len(l.g.Meta.Symbols) {
		return l.g.Meta.Symbols[s]
	}
	return ""
}

// FieldForName finds a field id by name.
func (l *Language) FieldForName(name string) (FieldID, bool) {
	l.index()
	f, ok := l.fields[name]
	return f, ok
}

// FieldName is the name of f ("" for id 0 or an unknown id).
func (l *Language) FieldName(f FieldID) string {
	if int(f) < len(l.g.Meta.Fields) {
		return l.g.Meta.Fields[f]
	}
	return ""
}
