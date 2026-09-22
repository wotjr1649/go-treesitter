package gpl_test

import (
	"context"
	"sync"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	_ "github.com/wotjr1649/go-treesitter/grammars/gpl"
	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestOptionalGrammars(t *testing.T) {
	cases := []struct{ name, filename, source string }{
		{"caddy", "Caddyfile", ":8080 {\n}\n"},
		{"disassembly", "example.dump", "0x400601 <__libc_csu_init+33>   sub    %r12,%rbp\n1000: 55                   push   rbp\n"},
		{"jq", "example.jq", "def f: . | length; .foo | f\n"},
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for _, c := range cases {
				for _, language := range []string{c.name, ""} {
					r, err := treesitter.New().Parse(context.Background(), syntax.Request{
						Language: language, Filename: c.filename, Source: []byte(c.source), Timeout: time.Second,
					})
					if r.Tree != nil {
						r.Tree.Close()
					}
					if err != nil || !r.Complete() || r.Outcome != syntax.AcceptedClean {
						t.Errorf("%s language=%q: %v %+v", c.name, language, err, r.Diagnostics)
					}
				}
			}
		})
	}
	workers.Wait()
}
