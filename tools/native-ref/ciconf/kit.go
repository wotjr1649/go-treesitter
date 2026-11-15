package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type kitManifest struct {
	Kind, Repository, Commit, ArchiveSHA256, License, Selection, Projection string
	SourceBudgetBytes                                                       int
	Cases                                                                   []struct {
		Lang, ID, File, FileSHA256, SourceSHA256 string
		Edits                                    []struct{ Find, Replace, OldSHA256, NewSHA256 string }
	}
}

type edit struct {
	id, lang, name, base, kind string
	old, new                   []byte
	start, oe, ne              int
	seed                       uint64
}

func projectKitEdit(src []byte, find, replace string) ([]byte, int, int, int, error) {
	at := bytes.Index(src, []byte(find))
	if find == "" {
		at = len(src)
	}
	if at < 0 {
		return nil, 0, 0, 0, fmt.Errorf("kit edit target not found")
	}
	nw := append(append(append([]byte{}, src[:at]...), replace...), src[at+len(find):]...)
	return nw, at, at + len(find), at + len(replace), nil
}

// loadKit checks every blob, including unselected shards, before returning any jobs.
func loadKit(dir string, selected []string) ([]job, []edit, string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, nil, "", err
	}
	var m kitManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return nil, nil, "", err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, nil, "", fmt.Errorf("trailing kit manifest data: %v", err)
	}
	if m.Kind != "kit-inputs/1" || m.Commit != "0a12156" || m.Repository != "https://github.com/wotjr1649/tree-sitter-grammar-kit" ||
		m.ArchiveSHA256 != "83d792c6453a5f316e101a76885f637b4397ba7a601cfebbd7a388f23cab07d3" ||
		m.Selection == "" || m.Projection == "" || m.SourceBudgetBytes != 262144 || len(m.Cases) == 0 {
		return nil, nil, "", fmt.Errorf("invalid kit provenance, rules or budget")
	}
	license, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil || string(license) != m.License || !strings.Contains(m.License, "Copyright (c) 2026 tree-sitter-grammar-kit contributors") {
		return nil, nil, "", fmt.Errorf("kit MIT notice mismatch: %v", err)
	}
	blobs := map[string][]byte{}
	total := 0
	read := func(hash string) ([]byte, error) {
		if b, ok := blobs[hash]; ok {
			return b, nil
		}
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return nil, fmt.Errorf("invalid source hash %q", hash)
		}
		b, err := os.ReadFile(filepath.Join(dir, "sources", hash+".in"))
		if err != nil {
			return nil, err
		}
		if sha(b) != hash {
			return nil, fmt.Errorf("kit source hash mismatch: %s", hash)
		}
		total += len(b)
		if total > m.SourceBudgetBytes {
			return nil, fmt.Errorf("kit source budget exceeded")
		}
		blobs[hash] = b
		return b, nil
	}
	var inputs []job
	var edits []edit
	ids, freshSeen, editSeen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	addInput := func(lang, file, id string, step int, b []byte) {
		key := lang + "/" + sha(b)
		if freshSeen[key] || !slices.Contains(selected, lang) {
			return
		}
		freshSeen[key] = true
		name := "kit-" + sha(b)
		inputs = append(inputs, job{lang: lang, id: lang + "/" + name, name: name, src: b,
			file: file, test: fmt.Sprintf("%s step %d", id, step), seq: step})
	}
	for _, c := range m.Cases {
		if !slices.Contains([]string{"csharp", "go", "json", "python", "tsql", "tsx", "typescript"}, c.Lang) || c.ID == "" || ids[c.Lang+"/"+c.ID] ||
			(c.File != "src/testdata/native/routes/"+c.Lang+".json" && c.File != "src/testdata/native/gaps/"+c.Lang+".json") ||
			len(c.FileSHA256) != 64 || strings.Trim(c.FileSHA256, "0123456789abcdef") != "" {
			return nil, nil, "", fmt.Errorf("invalid or duplicate kit case: %s/%s", c.Lang, c.ID)
		}
		ids[c.Lang+"/"+c.ID] = true
		old, err := read(c.SourceSHA256)
		if err != nil {
			return nil, nil, "", err
		}
		addInput(c.Lang, c.File, c.ID, 0, old)
		for i, e := range c.Edits {
			if sha(old) != e.OldSHA256 {
				return nil, nil, "", fmt.Errorf("%s step %d: preceding source mismatch", c.ID, i+1)
			}
			nw, start, oe, ne, err := projectKitEdit(old, e.Find, e.Replace)
			if err != nil {
				return nil, nil, "", fmt.Errorf("%s step %d: %w", c.ID, i+1, err)
			}
			stored, err := read(e.NewSHA256)
			if err != nil {
				return nil, nil, "", err
			}
			if !bytes.Equal(stored, nw) {
				return nil, nil, "", fmt.Errorf("%s step %d: projected source mismatch", c.ID, i+1)
			}
			addInput(c.Lang, c.File, c.ID, i+1, nw)
			key := fmt.Sprintf("%s/%s/%s/%d/%d/%d", c.Lang, e.OldSHA256, e.NewSHA256, start, oe, ne)
			if !editSeen[key] && slices.Contains(selected, c.Lang) {
				editSeen[key] = true
				name := "kit-" + sha([]byte(key))
				edits = append(edits, edit{id: c.Lang + "/" + name, lang: c.Lang, name: name,
					base: fmt.Sprintf("%s/%s#%d", c.Lang, c.ID, i+1), kind: "kit-pair", old: old, new: nw, start: start, oe: oe, ne: ne})
			}
			old = nw
		}
	}
	if ents, err := os.ReadDir(filepath.Join(dir, "sources")); err != nil || len(ents) != len(blobs) {
		return nil, nil, "", fmt.Errorf("kit sources/ holds files the manifest does not reference: %d files, %d referenced (%v)", len(ents), len(blobs), err)
	}
	return inputs, edits, sha(raw), nil
}
