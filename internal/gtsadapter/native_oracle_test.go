//go:build oracle_probe

package gtsadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestNativeOracleDifferential(t *testing.T) {
	path := os.Getenv("GTS_NATIVE_ORACLE_EXE")
	if path == "" {
		t.Fatal("GTS_NATIVE_ORACLE_EXE is required for this diagnostic lane")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path).Output()
	if err != nil {
		t.Fatalf("native oracle execution: %T", err)
	}
	type receipt struct {
		Fixture, Language, Source, SExpr string
		ABI                              int
		HasError                         bool `json:"has_error"`
		Start, End                       uint32
		Nodes                            []syntax.Node
	}
	var records []receipt
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var record receipt
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != len(regressionFixtures) {
		t.Fatal("oracle fixture set mismatch")
	}
	for i, c := range records {
		f := regressionFixtures[i]
		if c.Fixture != f.id || c.Source != f.source || c.Language != "tsx" || c.Start != 0 || c.End != uint32(len(f.source)) || len(c.Nodes) == 0 {
			t.Fatal("oracle input/completeness mismatch")
		}
		r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.tsx", Source: []byte(f.source)})
		if err != nil || !r.Complete() || r.Tree == nil {
			t.Fatalf("incomplete Go receipt: %+v", r.Diagnostics)
		}
		g := r.Tree.Nodes()
		r.Tree.Close()
		first := -1
		for j := 0; j < min(len(g), len(c.Nodes)); j++ {
			if !reflect.DeepEqual(g[j], c.Nodes[j]) {
				first = j
				break
			}
		}
		if first < 0 && len(g) != len(c.Nodes) {
			first = min(len(g), len(c.Nodes))
		}
		goJSON, _ := json.Marshal(g)
		cJSON, _ := json.Marshal(c.Nodes)
		t.Logf("%s source_sha256=%x go_digest=%x c_digest=%x go_len=%d c_len=%d first_difference=%d go=%+v c_has_error=%t c_sexpr=%s", f.id, sha256.Sum256([]byte(f.source)), sha256.Sum256(goJSON), sha256.Sum256(cJSON), len(g), len(c.Nodes), first, r.Diagnostics, c.HasError, c.SExpr)
		if first >= 0 {
			if first < len(g) {
				t.Logf("Go[%d]=%+v", first, g[first])
			}
			if first < len(c.Nodes) {
				t.Logf("C[%d]=%+v", first, c.Nodes[first])
			}
		}
		if c.HasError != (i < 3) || r.HasError != (i < 3) {
			t.Fatalf("unexpected error-state differential for %s", f.id)
		}
		for _, node := range c.Nodes {
			if i >= 3 && node.Missing {
				t.Fatalf("C clean expectation contains a missing node for %s", f.id)
			}
		}
		if i >= 3 && first != -1 {
			t.Fatalf("control tree differential for %s", f.id)
		}
	}
}
