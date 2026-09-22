//go:build !grammar_subset || grammar_subset_cobol

package gtsadapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/internal/provenance"
	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestCobolGrammarOwnedRootSpan(t *testing.T) {
	var records struct {
		RuntimeCommit string `json:"runtime_commit"`
		GrammarCommit string `json:"grammar_commit"`
		BlobSHA256    string `json:"blob_sha256"`
		Rows          []struct {
			Source       string
			SourceSHA256 string `json:"source_sha256"`
			C            struct {
				HasError bool `json:"has_error"`
				Nodes    []syntax.Node
			}
		}
	}
	data, err := os.ReadFile("../../testdata/oracle/catalog-cobol.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	pins, err := provenance.Read()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile("../runtime/grammars/grammar_blobs/cobol.bin")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(blob)) != records.BlobSHA256 ||
		records.RuntimeCommit != pins.Oracle.RuntimeCommit || records.GrammarCommit != "e99dbdc3d800d5fa2796476efd60af91f6b43d93" || len(records.Rows) != 3 {
		t.Fatal("COBOL oracle identity changed")
	}
	for i, record := range records.Rows {
		if record.SourceSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(record.Source))) || record.C.HasError {
			t.Fatal("invalid COBOL input record")
		}
		result, err := (Adapter{}).Parse(context.Background(), syntax.Request{Language: "cobol", Source: []byte(record.Source), Timeout: time.Second})
		if result.Tree != nil {
			defer result.Tree.Close()
		}
		if err != nil || !result.Complete() || result.Outcome != syntax.AcceptedClean || result.RootStartByte == 0 {
			t.Fatalf("case %d: complete COBOL result rejected: %+v", i, result.Diagnostics)
		}
		if !reflect.DeepEqual(result.Tree.Nodes(), record.C.Nodes) {
			t.Fatalf("case %d: Go=%+v C=%+v", i, result.Tree.Nodes(), record.C.Nodes)
		}
	}
}
