package treesitter

import (
	"runtime"
	"sync"
	"weak"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

type cachedQuery struct {
	ptr     int32
	cleanup runtime.Cleanup
}

// Each instance has its own queue, so a late cleanup cannot enqueue work for its replacement.
// The callback holds the queue and a weak key without retaining the instance or Query.
type queryRetirements struct {
	mu      sync.Mutex
	pending []weak.Pointer[Query]
}

type queryRetirement struct {
	queue *queryRetirements
	key   weak.Pointer[Query]
}

func retireQuery(arg queryRetirement) {
	r := arg.queue
	r.mu.Lock()
	r.pending = append(r.pending, arg.key)
	r.mu.Unlock()
}

func (in *instance) stopQueryCleanups() {
	for key, entry := range in.queries {
		if q := key.Value(); q != nil {
			entry.cleanup.Stop()
			runtime.KeepAlive(q)
		}
	}
}

// drainQueries runs under the busy guard on the owner goroutine, never in a GC cleanup.
func (c *core) drainQueries() error {
	in := c.inst
	r := in.queryQueue
	if r == nil {
		return nil
	}
	r.mu.Lock()
	pending := r.pending
	r.pending = nil
	r.mu.Unlock()
	for _, key := range pending {
		entry, ok := in.queries[key]
		if !ok {
			continue
		}
		delete(in.queries, key)
		if err := c.queryCall("tsg_query_delete", func() { in.m.(engine.QueryModule).Xtsg_query_delete(entry.ptr) }); err != nil {
			return err
		}
	}
	return nil
}
