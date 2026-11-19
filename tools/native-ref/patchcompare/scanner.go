package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"unicode"
)

type scannerBuild struct {
	Grammar     string          `json:"grammar"`
	Identity    json.RawMessage `json:"identity"`
	Comparisons []scannerPair   `json:"comparisons"`
	Platform    string          `json:"platform"`
	Compiler    string          `json:"compiler"`
}

type scannerPair struct {
	Patch               string `json:"patch"`
	Before              string `json:"before"`
	After               string `json:"after"`
	BeforeSHA256        string `json:"beforeSHA256"`
	AfterSHA256         string `json:"afterSHA256"`
	BeforeSourcesSHA256 string `json:"beforeSourcesSHA256"`
	AfterSourcesSHA256  string `json:"afterSourcesSHA256"`
}

type scannerExpectation struct {
	Version      int                 `json:"version"`
	Grammar      string              `json:"grammar"`
	Identity     json.RawMessage     `json:"identity"`
	InputsSHA256 string              `json:"inputsSHA256"`
	Comparisons  []scannerComparison `json:"comparisons"`
}

type scannerComparison struct {
	Patch               string              `json:"patch"`
	BeforeSourcesSHA256 string              `json:"beforeSourcesSHA256"`
	AfterSourcesSHA256  string              `json:"afterSourcesSHA256"`
	ResultsSHA256       string              `json:"resultsSHA256"`
	Reason              string              `json:"reason"`
	RetireWhen          string              `json:"retireWhen"`
	Differences         []scannerDifference `json:"differences"`
}

type scannerInput struct {
	ID     string `json:"id"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type scannerOutcome struct {
	Status string `json:"status"`
	Tree   *tree  `json:"tree,omitempty"`
	// Crash is the platform-specific class of a baseline crash; rows keep only the portable status.
	Crash string `json:"crash,omitempty"`
}

type scannerResult struct {
	scannerInput
	Before scannerOutcome `json:"before"`
	After  scannerOutcome `json:"after"`
}

type scannerDifference struct {
	scannerResult
	// BaselineCrashes maps each reviewed platform (GOOS/GOARCH) to the crash class of the original scanner.
	BaselineCrashes map[string]string `json:"baselineCrashes,omitempty"`
	Review          string            `json:"review,omitempty"`
}

type scannerReport struct {
	Build    *scannerBuild      `json:"build,omitempty"`
	Inputs   []scannerInput     `json:"inputs,omitempty"`
	Actual   scannerExpectation `json:"actual"`
	Results  [][]scannerResult  `json:"results,omitempty"`
	Failures []string           `json:"failures,omitempty"`
	Mode     string             `json:"mode"`
	Pass     bool               `json:"pass"`
}

func runScanner(buildFile, expectations, candidate, corpus, source, regressions, out string) (runErr error) {
	if (expectations == "") == (candidate == "") || out == "" {
		return fmt.Errorf("scanner mode requires -out and exactly one of -expectations or -write-candidate")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return fmt.Errorf("create new evidence directory: %w", err)
	}
	reportPath := filepath.Join(out, "report.json")
	reportFile, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	r := scannerReport{Mode: "check"}
	if candidate != "" {
		r.Mode = "candidate"
	}
	defer func() {
		r.Pass = runErr == nil
		if runErr != nil {
			r.Failures = append(r.Failures, runErr.Error())
		}
		encoder := json.NewEncoder(reportFile)
		encoder.SetIndent("", "  ")
		runErr = errors.Join(runErr, encoder.Encode(r), reportFile.Close())
		fmt.Printf("scanner grammar=%s mode=%s pairs=%d inputs=%d pass=%t report=%s\n", r.Actual.Grammar, r.Mode, len(r.Actual.Comparisons), len(r.Inputs), runErr == nil, reportPath)
	}()

	var build scannerBuild
	if err := readScannerJSON(buildFile, &build); err != nil {
		return err
	}
	r.Build = &build
	r.Actual = scannerExpectation{Version: 1, Grammar: build.Grammar, Comparisons: []scannerComparison{}}
	if err := validateScannerBuild(build); err != nil {
		return err
	}
	if len(build.Comparisons) == 0 {
		if candidate != "" {
			return fmt.Errorf("%s has no active scanner patches", build.Grammar)
		}
		if _, err := os.Lstat(expectations); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		return fmt.Errorf("stale scanner expectations for %s: %s exists with no active scanner patches", build.Grammar, expectations)
	}
	identity, err := canonicalIdentity(build.Identity)
	if err != nil {
		return err
	}
	r.Actual.Identity = identity
	if candidate != "" {
		if _, err := os.Lstat(candidate); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("candidate file must be new: %s", candidate)
		}
	}
	if corpus == "" || source == "" || regressions == "" {
		return fmt.Errorf("active scanner comparisons require -corpus, -fuzz-source and -regressions")
	}
	inputs, err := scannerInputs(build.Grammar, corpus, source, regressions)
	if err != nil {
		return err
	}
	for _, in := range inputs {
		entry := scannerInput{in.ID, len(in.Bytes), sha(in.Bytes)}
		r.Inputs = append(r.Inputs, entry)
		if err := write(scannerArtifact(out, "inputs", in.ID, ".in"), in.Bytes); err != nil {
			return err
		}
	}
	r.Actual.InputsSHA256 = scannerDigest(r.Inputs)
	for idx, pair := range build.Comparisons {
		comparison := scannerComparison{
			Patch: pair.Patch, BeforeSourcesSHA256: pair.BeforeSourcesSHA256,
			AfterSourcesSHA256: pair.AfterSourcesSHA256, Differences: []scannerDifference{},
		}
		pairOut := filepath.Join(out, "pairs", fmt.Sprintf("%03d", idx+1))
		rows := make([]scannerResult, 0, len(inputs))
		for _, in := range r.Inputs {
			path := scannerArtifact(out, "inputs", in.ID, ".in")
			before, err := scannerDump(pair.Before, path, scannerArtifact(pairOut, "before", in.ID, ".dump"), in.Bytes, true)
			if err != nil {
				r.Failures = append(r.Failures, fmt.Sprintf("%s %s before: %v", pair.Patch, in.ID, err))
			}
			after, err := scannerDump(pair.After, path, scannerArtifact(pairOut, "after", in.ID, ".dump"), in.Bytes, false)
			if err != nil {
				r.Failures = append(r.Failures, fmt.Sprintf("%s %s after: %v", pair.Patch, in.ID, err))
			}
			crash := before.Crash
			before.Crash = ""
			row := scannerResult{in, before, after}
			rows = append(rows, row)
			if !reflect.DeepEqual(before, after) {
				difference := scannerDifference{scannerResult: row}
				if crash != "" {
					difference.BaselineCrashes = map[string]string{build.Platform: crash}
				}
				comparison.Differences = append(comparison.Differences, difference)
			}
			if len(r.Failures) != 0 {
				break
			}
		}
		comparison.ResultsSHA256 = scannerDigest(rows)
		r.Actual.Comparisons = append(r.Actual.Comparisons, comparison)
		r.Results = append(r.Results, rows)
		if len(r.Failures) != 0 {
			return fmt.Errorf("%d native comparison failures (first: %s)", len(r.Failures), r.Failures[0])
		}
	}
	if candidate != "" {
		return writeScannerCandidate(candidate, r.Actual)
	}
	var expected scannerExpectation
	if err := readScannerJSON(expectations, &expected); err != nil {
		return err
	}
	return checkScannerExpectations(expected, r.Actual)
}

func scannerArtifact(dir, kind, id, ext string) string {
	return filepath.Join(dir, kind, sha([]byte(id))+ext)
}

func validateScannerBuild(b scannerBuild) error {
	if b.Grammar == "" || strings.IndexFunc(b.Grammar, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) >= 0 || b.Platform != runtime.GOOS+"/"+runtime.GOARCH || b.Comparisons == nil {
		return fmt.Errorf("incomplete scanner build manifest or incorrect platform (%s)", b.Platform)
	}
	if len(b.Comparisons) != 0 && strings.TrimSpace(b.Compiler) == "" {
		return fmt.Errorf("scanner build manifest missing compiler version")
	}
	seen := map[string]bool{}
	for _, pair := range b.Comparisons {
		if !strings.HasPrefix(pair.Patch, "patches/"+b.Grammar+"/") || seen[pair.Patch] {
			return fmt.Errorf("invalid or duplicate scanner patch %q", pair.Patch)
		}
		seen[pair.Patch] = true
		if !validSHA256(pair.BeforeSourcesSHA256) || !validSHA256(pair.AfterSourcesSHA256) {
			return fmt.Errorf("invalid source digest for %s", pair.Patch)
		}
		for _, exe := range []struct{ path, hash string }{{pair.Before, pair.BeforeSHA256}, {pair.After, pair.AfterSHA256}} {
			if !filepath.IsAbs(exe.path) {
				return fmt.Errorf("scanner executable path must be absolute: %q", exe.path)
			}
			hash, err := scannerFileSHA256(exe.path)
			if err != nil {
				return err
			}
			if hash != exe.hash {
				return fmt.Errorf("scanner executable digest mismatch: %s", exe.path)
			}
		}
	}
	return nil
}

func scannerFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validSHA256(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 64 && err == nil && strings.ToLower(s) == s
}

func checkScannerExpectations(want, got scannerExpectation) error {
	wantID, err := canonicalIdentity(want.Identity)
	if err != nil {
		return err
	}
	if want.Version != got.Version || want.Grammar != got.Grammar || !bytes.Equal(wantID, got.Identity) {
		return fmt.Errorf("scanner expectation build identity mismatch for %s", got.Grammar)
	}
	if want.InputsSHA256 != got.InputsSHA256 {
		return fmt.Errorf("%s scanner input digest mismatch: expected %s, got %s", got.Grammar, want.InputsSHA256, got.InputsSHA256)
	}
	if len(want.Comparisons) != len(got.Comparisons) {
		return fmt.Errorf("expected %d scanner patches, found %d", len(want.Comparisons), len(got.Comparisons))
	}
	for i, pair := range got.Comparisons {
		expected := want.Comparisons[i]
		if expected.Patch != pair.Patch || expected.BeforeSourcesSHA256 != pair.BeforeSourcesSHA256 || expected.AfterSourcesSHA256 != pair.AfterSourcesSHA256 {
			return fmt.Errorf("scanner comparison %d identity mismatch for %s", i+1, pair.Patch)
		}
		if strings.TrimSpace(expected.Reason) == "" || strings.TrimSpace(expected.RetireWhen) == "" {
			return fmt.Errorf("%s needs a reviewed reason and retirement condition", pair.Patch)
		}
		if err := checkScannerDifferences(expected.Differences, pair.Differences); err != nil {
			return fmt.Errorf("%s: %w", pair.Patch, err)
		}
		if expected.ResultsSHA256 != pair.ResultsSHA256 {
			return fmt.Errorf("%s all-results digest mismatch: expected %s, got %s", pair.Patch, expected.ResultsSHA256, pair.ResultsSHA256)
		}
	}
	return nil
}

func checkScannerDifferences(want, got []scannerDifference) error {
	if want == nil {
		return fmt.Errorf("missing differences array")
	}
	for i, row := range want {
		if row.ID == "" || (i > 0 && want[i-1].ID >= row.ID) {
			return fmt.Errorf("difference IDs empty, duplicated or out of order at %q", row.ID)
		}
	}
	for i := 0; i < max(len(want), len(got)); i++ {
		if i == len(want) || (i < len(got) && got[i].ID < want[i].ID) {
			return fmt.Errorf("new difference for %q", got[i].ID)
		}
		if i == len(got) || want[i].ID < got[i].ID {
			return fmt.Errorf("missing difference for %q", want[i].ID)
		}
		if !reflect.DeepEqual(want[i].scannerResult, got[i].scannerResult) {
			return fmt.Errorf("changed difference for %q", got[i].ID)
		}
		if (want[i].Before.Status == "baseline-crash") != (len(want[i].BaselineCrashes) != 0) {
			return fmt.Errorf("baseline crash of %q needs per-platform crash classes", got[i].ID)
		}
		for platform, class := range got[i].BaselineCrashes {
			if want[i].BaselineCrashes[platform] != class {
				return fmt.Errorf("unreviewed %s baseline crash on %s for %q", class, platform, got[i].ID)
			}
		}
		before := got[i].Before
		if (before.Tree == nil || !before.Tree.HasError) && strings.TrimSpace(want[i].Review) == "" {
			return fmt.Errorf("missing review of clean change or baseline crash for %q", got[i].ID)
		}
	}
	return nil
}

func canonicalIdentity(raw json.RawMessage) (json.RawMessage, error) {
	var id map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&id); err != nil || len(id) == 0 {
		return nil, fmt.Errorf("scanner build identity must be a nonempty JSON object")
	}
	return json.Marshal(id)
}

func scannerDigest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // Only concrete scanner input and result rows are passed here.
	}
	return sha(b)
}

func writeScannerCandidate(path string, v scannerExpectation) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}

func readScannerJSON(path string, dest any) error {
	b, err := os.ReadFile(path)
	if err == nil {
		err = decodeScannerJSON(b, dest, true)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func decodeScannerJSON(b []byte, dest any, strict bool) error {
	// encoding/json alone silently accepts duplicate object keys.
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := scanJSONValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("unexpected content after JSON value")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	if strict {
		d.DisallowUnknownFields()
	}
	return d.Decode(dest)
}

func scanJSONValue(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			tok, err := d.Token()
			if err != nil {
				return err
			}
			key := tok.(string)
			folded := strings.Map(func(r rune) rune {
				for next := unicode.SimpleFold(r); next > r; next = unicode.SimpleFold(r) {
					r = next
				}
				return unicode.SimpleFold(r)
			}, key)
			if seen[folded] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[folded] = true
			if err := scanJSONValue(d); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for d.More() {
			if err := scanJSONValue(d); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = d.Token()
	return err
}

func sortedScannerInputs(inputs []input) ([]input, error) {
	slices.SortFunc(inputs, func(a, b input) int { return cmp.Compare(a.ID, b.ID) })
	for i, in := range inputs {
		if strings.HasSuffix(in.ID, "/") || (i > 0 && inputs[i-1].ID == in.ID) {
			return nil, fmt.Errorf("empty or duplicate scanner input ID %q", in.ID)
		}
	}
	return inputs, nil
}
