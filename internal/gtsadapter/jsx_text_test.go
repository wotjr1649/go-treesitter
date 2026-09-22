package gtsadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestJSXEqualsEdits(t *testing.T) {
	for _, language := range []struct{ name, filename string }{{"tsx", "x.tsx"}, {"javascript", "x.jsx"}} {
		for _, f := range regressionFixtures[3:5] {
			t.Run(language.name+"/"+f.id, func(t *testing.T) {
				after := []byte(f.source)
				before := bytes.Replace(after, []byte("a = b"), []byte("a b"), 1)
				before = bytes.Replace(before, []byte("k=v"), []byte("k v"), 1)
				start := 0
				for start < min(len(before), len(after)) && before[start] == after[start] {
					start++
				}
				oldEnd, newEnd := len(before), len(after)
				for oldEnd > start && newEnd > start && before[oldEnd-1] == after[newEnd-1] {
					oldEnd--
					newEnd--
				}
				first, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: language.filename, Source: before, Timeout: 10 * time.Second})
				if first.Tree != nil {
					defer first.Tree.Close()
				}
				if err != nil || !first.Complete() || first.Outcome != syntax.AcceptedClean {
					t.Fatal("initial edit tree incomplete")
				}
				edit := &syntax.Edit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd)}
				inc, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: language.filename, Source: after, Previous: first.Tree, Edit: edit, Timeout: 10 * time.Second})
				if inc.Tree != nil {
					defer inc.Tree.Close()
				}
				if err != nil || !inc.Complete() || inc.Outcome != syntax.AcceptedClean {
					t.Fatalf("JSX incremental incomplete: %+v", inc.Diagnostics)
				}
				data, err := os.ReadFile("../../testdata/oracle/windows-c-v2/base/" + language.name + "-" + f.id + ".json")
				if err != nil {
					t.Fatal(err)
				}
				var c oracleRecord
				if err := json.Unmarshal(data, &c); err != nil {
					t.Fatal(err)
				}
				if nodeDigest(inc.Tree.Nodes()) != c.NodesSHA256 {
					t.Fatal("JSX incremental C tree difference")
				}
				t.Logf("fixture=%s-%s source=%s C=%s incremental=%s", language.name, f.id, c.SourceSHA256, c.NodesSHA256, nodeDigest(inc.Tree.Nodes()))
			})
		}
	}
}
