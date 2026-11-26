package main

// Runtime-only rehearsal (TEST ONLY): generate grammar modules against a different tree-sitter runtime source while every
// other pin (CLI, wasm2go, wasi-sdk, shim, grammar sources, patches) stays the committed one. `gen runtime-pins` writes the
// pins copy; `gen -runtime-rehearsal <copy>` reads it, and only into a new empty -out outside grammars/ (the output
// boundary of -balance-probe). Neither touches pins.json or any committed module. Driver: tools/gen/runtime-rehearsal.sh.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
)

const testOnlyRuntimeRehearsal = "runtime-rehearsal"

var (
	commitRx  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	releaseRx = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`)
)

func readPinsMap(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// checkRuntimeOnlyPins accepts a pins copy only if it equals the committed pins.json outside the "runtime" block.
func checkRuntimeOnlyPins(committed, override string) error {
	a, err := readPinsMap(committed)
	if err != nil {
		return err
	}
	b, err := readPinsMap(override)
	if err != nil {
		return err
	}
	if _, ok := b["runtime"]; !ok {
		return fmt.Errorf("%s has no runtime block", override)
	}
	delete(a, "runtime")
	delete(b, "runtime")
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("%s differs from %s outside the runtime block: a runtime rehearsal moves nothing else", override, committed)
	}
	return nil
}

// runRuntimePinsCommand: gen runtime-pins -root tools/gen -src <dir> -extract-dir <name> -archive <file> -release <label>
// -commit <sha> -out <new file>. It hashes the downloaded runtime archive and its extracted tree and writes a pins copy
// whose runtime block names them. The caller downloaded the archive; nothing here fetches or executes anything.
func runRuntimePinsCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("runtime-pins", flag.ContinueOnError)
	root := fs.String("root", ".", "tools/gen directory (committed pins.json)")
	src := fs.String("src", "", "directory holding the extracted runtime tree")
	extractDir := fs.String("extract-dir", "", "directory name of the extracted tree under -src")
	archive := fs.String("archive", "", "downloaded runtime source archive (file)")
	release := fs.String("release", "", "label recorded as the runtime identity (tag, or a label of your choice)")
	commit := fs.String("commit", "", "full 40-hex commit of the runtime source")
	out := fs.String("out", "", "pins copy to write (must not exist, must not be <root>/pins.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *src == "" || *archive == "" || *out == "" {
		return errors.New("-src, -archive and -out are required")
	}
	if !commitRx.MatchString(*commit) {
		return fmt.Errorf("-commit %q is not a full lowercase 40-hex commit", *commit)
	}
	if !releaseRx.MatchString(*release) {
		return fmt.Errorf("-release %q must be letters, digits, dot, dash or underscore", *release)
	}
	if !filepath.IsLocal(*extractDir) || filepath.Base(*extractDir) != *extractDir {
		return fmt.Errorf("-extract-dir %q must be one directory name", *extractDir)
	}
	committed := filepath.Join(*root, "pins.json")
	pins, err := loadPins(committed)
	if err != nil {
		return err
	}
	rt := pins.Runtime
	rt.Release, rt.Commit, rt.ExtractDir, rt.Archive = *release, *commit, *extractDir, filepath.Base(*archive)
	if rt.ArchiveSHA256, err = sumFile(*archive); err != nil {
		return err
	}
	if rt.TreeSHA256, err = treeSHA256(filepath.Join(*src, rt.ExtractDir), rt.Dirs); err != nil {
		return err
	}
	if rt.LicenseSHA256, err = sumFile(filepath.Join(*src, rt.ExtractDir, "LICENSE")); err != nil {
		return err
	}
	m, err := readPinsMap(committed)
	if err != nil {
		return err
	}
	m["runtime"] = rt
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if same, err := sameFile(committed, *out); err != nil {
		return err
	} else if same {
		return errors.New("-out must not be the committed pins.json")
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(rt)
}

// sameFile reports whether b names the same file as a (a missing b is a different file).
func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}
