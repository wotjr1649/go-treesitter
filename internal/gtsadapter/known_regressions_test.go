package gtsadapter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

// Exact inline bytes and same-walker node counts from the regression register.
var regressionFixtures = []struct {
	id, source      string
	tsx, javascript int
}{
	{"F1", "const a = <p>Org & Team</p>;", 16, 18},
	{"F2", "const b = <p>AT&T</p>;", 16, 18},
	{"F3", "const c = <p>&</p>;", 18, 17},
	{"F4", "const d = <p>a = b</p>;", 17, 15},
	{"F5", "const e = <code>k=v</code>;", 17, 15},
	{"F6", "const f = <p>x &amp; y</p>;", 19, 19},
	{"F7", "const g = <p>plain</p>;", 17, 17},
}

func TestKR0001aCharacterization(t *testing.T)    { regressionCases(t, 0, 3, "KR-0001a") }
func TestKR0001bSuspectedDivergence(t *testing.T) { regressionCases(t, 3, 5, "KR-0001b") }
func TestKR0001Controls(t *testing.T)             { regressionCases(t, 5, 7, "controls") }

func regressionCases(t *testing.T, start, end int, record string) {
	t.Helper()
	if len(regressionFixtures) != 7 {
		t.Fatal("provenance failure: regression fixture missing")
	}
	for i, f := range regressionFixtures {
		if f.id != fmt.Sprintf("F%d", i+1) {
			t.Fatal("provenance failure: fixture order/identity")
		}
	}
	for _, f := range regressionFixtures[start:end] {
		for _, file := range []string{"x.tsx", "x.jsx"} {
			t.Run(f.id+"/"+file, func(t *testing.T) {
				r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: file, Source: []byte(f.source)})
				if r.Tree != nil {
					defer r.Tree.Close()
				}
				count := 0
				if r.Tree != nil {
					count = len(r.Tree.Nodes())
				}
				t.Logf("record=%s fixture=%s sha256=%x node_count=%d diagnostics=%+v", record, f.id, sha256.Sum256([]byte(f.source)), count, r.Diagnostics)
				wantCount := f.tsx
				if file == "x.jsx" {
					wantCount = f.javascript
				}
				if record == "controls" {
					if err != nil || r.Outcome != syntax.AcceptedClean || !r.Complete() || count != wantCount {
						t.Fatal("REGRESSION EXPANSION: clean control changed")
					}
					return
				}
				if r.Outcome == syntax.AcceptedClean {
					if record == "KR-0001a" {
						t.Fatal("NEW REGRESSION KR-0001a: bare ampersand became clean; oracle-characterization drift")
					}
					t.Fatal("STALE KR-0001b: equals failure disappeared; investigate and retire the record")
				}
				if err != nil || r.Outcome != syntax.AcceptedWithErrors || !r.Complete() || !r.HasError || count != wantCount {
					t.Fatalf("NEW REGRESSION %s: signature changed; expected %d nodes", record, wantCount)
				}
			})
		}
	}
}
