package main

import (
	"bytes"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

// TSG_PINBUMP_LIVE=1 re-derives every owner pin of the committed pins.json from the real GitHub API tarball of its pinned
// commit (read-only GET, nothing from the archive runs) and requires the result to equal the committed fields. It
// is the evidence that pinbump computes the Pins procedure's fields the way the committed pins were computed.
func TestLiveReproducesCommittedPins(t *testing.T) {
	if os.Getenv("TSG_PINBUMP_LIVE") != "1" {
		t.Skip("set TSG_PINBUMP_LIVE=1 (network)")
	}
	pins, err := os.ReadFile("../pins.json")
	if err != nil {
		t.Fatal(err)
	}
	c := config{API: defaultAPI, Client: &http.Client{Timeout: 5 * time.Minute}, Token: os.Getenv("GITHUB_TOKEN")}
	for _, name := range []string{"java", "javascript", "typescript", "tsql"} {
		group, err := groupOf(pins, name)
		if err != nil {
			t.Fatal(err)
		}
		m := repoRE.FindStringSubmatch(group[0].Old.Repo)
		var hashed []string
		for _, g := range group {
			hashed = append(hashed, g.Old.Dirs...)
		}
		keep := func(rel string) bool {
			return rel == "LICENSE" || rel == "NOTICE" || rel == "CHANGELOG.md" || hasDir(hashed, rel)
		}
		b, code, err := c.get(c.API+"/repos/"+m[1]+"/tarball/"+group[0].Old.Commit, maxArchive)
		if err != nil || code != http.StatusOK {
			t.Fatalf("%s: tarball %d %v", name, code, err)
		}
		files, err := readArchive(bytes.NewReader(b), keep, hashed)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range group {
			got, err := derive(g.Old, m[2], g.Old.Commit, sum(b), files)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, g.Name, err)
			}
			if !reflect.DeepEqual(got, g.Old) {
				t.Errorf("%s/%s: derived %+v, committed %+v", name, g.Name, got, g.Old)
			} else {
				t.Logf("%s/%s: all %d pin fields reproduced", name, g.Name, 9+len(g.Old.Files))
			}
		}
		_, hasChangelog := files["CHANGELOG.md"]
		t.Logf("%s: CHANGELOG.md at the pinned commit: %v", name, hasChangelog)
	}
}
