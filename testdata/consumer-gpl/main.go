package main

import (
	"context"
	"fmt"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	_ "github.com/wotjr1649/go-treesitter/grammars/gpl"
	"github.com/wotjr1649/go-treesitter/syntax"
)

func main() {
	for name, source := range map[string]string{
		"caddy": ":8080 {\n}\n", "disassembly": "1000: 55                   push   rbp\n",
		"jq": "def f: . | length; .foo | f\n", "go": "package main\nfunc main() {}\n",
	} {
		r, err := treesitter.New().Parse(context.Background(), syntax.Request{
			Language: name, Source: []byte(source), Timeout: time.Second,
		})
		if err != nil || !r.Complete() || r.Outcome != syntax.AcceptedClean || r.Tree == nil {
			panic(fmt.Sprintf("%s: %v %+v", name, err, r.Diagnostics))
		}
		r.Tree.Close()
	}
	fmt.Println("external GPL consumer: caddy, disassembly, jq, go OK")
}
