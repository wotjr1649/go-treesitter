package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func scannerInputs(lang, corpus, source, regressions string) ([]input, error) {
	inputs, err := corpusInputs(corpus)
	if err != nil {
		return nil, err
	}
	fuzz, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	target := "FuzzParse" + strings.ToUpper(lang[:1]) + lang[1:]
	switch lang {
	case "javascript":
		target = "FuzzParseJavaScript"
	case "yaml":
		target = "FuzzParseYAML"
	case "xml":
		target = "FuzzParseXML"
	}
	seeds, err := fuzzSeeds(source, fuzz, target)
	if err != nil {
		return nil, err
	}
	inputs = appendFuzzInputs(inputs, seeds)

	// Ruby also compares its grammar-text fixtures, which carry the symbol regressions.
	if lang == "rust" || lang == "javascript" || lang == "xml" || lang == "ruby" {
		path := filepath.Join(regressions, "grammar-text-patches.json")
		var fixtures map[string][]struct{ Name, Source string }
		b, err := os.ReadFile(path)
		if err == nil {
			err = decodeScannerJSON(b, &fixtures, false)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(fixtures[lang]) == 0 {
			return nil, fmt.Errorf("no %s scanner regression inputs in %s", lang, path)
		}
		for _, f := range fixtures[lang] {
			inputs = append(inputs, input{"regression/" + f.Name, []byte(f.Source)})
		}
	}
	if lang != "rust" && lang != "javascript" && lang != "xml" {
		files := []string{lang + ".json"}
		switch lang {
		case "yaml", "python":
			files[0] += ".gz"
		case "bash", "ruby":
			files = append(files, lang+"-heredoc.json")
		}
		for _, file := range files {
			path := filepath.Join(regressions, "scanner-patches", file)
			cases, err := readScannerFixtures(path)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			prefix := "regression/"
			if strings.HasSuffix(file, "-heredoc.json") {
				prefix = "heredoc/"
			}
			for name, b := range cases {
				inputs = append(inputs, input{prefix + name, b})
			}
		}
	}
	return sortedScannerInputs(inputs)
}

func readScannerFixtures(path string) (map[string][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		z, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		reader = z
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	var cases map[string][]byte
	if err := decodeScannerJSON(b, &cases, false); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no scanner regression inputs")
	}
	for name, src := range cases {
		if name == "" || src == nil {
			return nil, fmt.Errorf("empty name or missing bytes for %q", name)
		}
	}
	return cases, nil
}
