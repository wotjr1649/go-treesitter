package gtsadapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestRecoveredErrorsCannotLookClean(t *testing.T) {
	manifestBytes, err := os.ReadFile("../../testdata/newtonsoft/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Fixtures []struct{ Name, SHA256 string }
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	const name = "JsonTextReader-excerpt.cs"
	source, err := os.ReadFile("../../testdata/newtonsoft/" + name)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, f := range manifest.Fixtures {
		if f.Name == name {
			matched = fmt.Sprintf("%x", sha256.Sum256(source)) == f.SHA256
		}
	}
	if !matched {
		t.Fatal("corpus provenance mismatch")
	}
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: name, Source: source})
	if r.Tree != nil {
		defer r.Tree.Close()
	}
	t.Logf("source_sha256=%x diagnostics=%+v", sha256.Sum256(source), r.Diagnostics)
	if err != nil || r.Tree == nil || r.Outcome != syntax.AcceptedWithErrors || r.HasMissing || !r.HasError || !r.Complete() {
		t.Fatalf("C recovery receipt changed: error=%T", err)
	}
	for _, n := range r.Tree.Nodes() {
		if n.Error || n.Missing {
			t.Logf("recovered=%+v", n)
		}
	}
	if r.Tree.(*tree).raw != nil {
		t.Fatal("non-clean backend tree retained")
	}
}
