package treesitter

import (
	"runtime"
	"weak"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

type queryRetirement struct {
	key      weak.Pointer[Query]
	instance weak.Pointer[instance]
}
type queryCleanup struct {
	owner weak.Pointer[core]
	queryRetirement
}

// The cleanup holds only weak references; neither a Query nor an abandoned parser is kept alive.
func retireQuery(a queryCleanup) {
	if c := a.owner.Value(); c != nil {
		c.mu.Lock()
		c.leakedQueries = append(c.leakedQueries, a.queryRetirement)
		c.mu.Unlock()
	}
}

// drainQueries runs under the busy guard on the owner goroutine, never in a GC cleanup.
func (c *core) drainQueries() error {
	c.mu.Lock()
	pending := c.leakedQueries
	c.leakedQueries = nil
	c.mu.Unlock()
	in := c.inst
	token := weak.Make(in)
	for _, r := range pending {
		if r.instance != token {
			continue
		}
		ptr, ok := in.queries[r.key]
		if !ok {
			continue
		}
		delete(in.queries, r.key)
		if err := c.queryCall("tsg_query_delete", func() { in.m.(engine.QueryModule).Xtsg_query_delete(ptr) }); err != nil {
			return err
		}
	}
	runtime.KeepAlive(in)
	return nil
}
