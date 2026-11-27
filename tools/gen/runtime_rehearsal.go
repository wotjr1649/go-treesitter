package main

// Runtime-only rehearsal (TEST ONLY): generate grammar modules against a different tree-sitter runtime source while every
// other pin (CLI, wasm2go, wasi-sdk, shim, grammar sources, patches) stays the committed one. `gen runtime-pins` writes the
// pins copy; `gen -runtime-rehearsal <copy>` reads it, and only into a new empty -out outside grammars/ (the output
// boundary of -balance-probe). Neither touches pins.json or any committed module. Driver: tools/gen/runtime-rehearsal.sh.
//
// Runtime + CLI pair (same files, still TEST ONLY): `gen runtime-pins -cli-version -cli-archive` also replaces the CLI version and
// this host's CLI archive in the copy. The candidate CLI is not pinned by anything but the bytes the copy hashes, so `gen`
// runs it only through -cli-sandbox (tools/gen/nightly-sandbox.sh, the repository's one path for an unpinned CLI); see pair_cli.go.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
)

const testOnlyRuntimeRehearsal = "runtime-rehearsal"

// cliAssetBase is where the pinned CLI archives come from; a candidate CLI is a release asset of the same repository.
const cliAssetBase = "https://github.com/tree-sitter/tree-sitter/releases/download/"

var (
	cliVersionRx = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)
	commitRx     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	releaseRx    = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`)
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

func stringMap(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

// checkRehearsalPins accepts a pins copy only if it equals the committed pins.json outside the "runtime" block, with one
// exception: a copy that moves the CLI (tools.treeSitterCLI or tools.treeSitterCLIArchives differ) may also differ in those two
// keys, which must then be set, and may drop grammars' upstreamTestException (it is bound to the committed CLI version, see
// loadPins; the real move has to re-decide it). cliMoved tells the caller that the candidate CLI must run sandboxed.
func checkRehearsalPins(committed, override string) (cliMoved bool, err error) {
	a, err := readPinsMap(committed)
	if err != nil {
		return false, err
	}
	b, err := readPinsMap(override)
	if err != nil {
		return false, err
	}
	if _, ok := b["runtime"]; !ok {
		return false, fmt.Errorf("%s has no runtime block", override)
	}
	delete(a, "runtime")
	delete(b, "runtime")
	ta, tb := stringMap(a, "tools"), stringMap(b, "tools")
	if ta != nil && tb != nil {
		cliMoved = !reflect.DeepEqual(ta["treeSitterCLI"], tb["treeSitterCLI"]) || !reflect.DeepEqual(ta["treeSitterCLIArchives"], tb["treeSitterCLIArchives"])
	}
	if cliMoved {
		if v, _ := tb["treeSitterCLI"].(string); v == "" || len(stringMap(tb, "treeSitterCLIArchives")) == 0 {
			return false, fmt.Errorf("%s moves the CLI without a version and an archive", override)
		}
		delete(ta, "treeSitterCLI")
		delete(tb, "treeSitterCLI")
		delete(ta, "treeSitterCLIArchives")
		delete(tb, "treeSitterCLIArchives")
		ga, gb := stringMap(a, "grammars"), stringMap(b, "grammars")
		for name := range ga {
			x, _ := ga[name].(map[string]any)
			y, _ := gb[name].(map[string]any)
			if _, ok := y["upstreamTestException"]; !ok && x != nil && y != nil {
				delete(x, "upstreamTestException")
			}
		}
	}
	if !reflect.DeepEqual(a, b) {
		return false, fmt.Errorf("%s differs from %s outside the runtime block%s: a rehearsal moves the runtime, and the CLI only through gen runtime-pins -cli-*", override, committed,
			map[bool]string{true: " and the CLI pin", false: ""}[cliMoved])
	}
	return cliMoved, nil
}

// cliIdentity is what a copy that moves the CLI records next to the runtime identity (runtime-identity.json).
type cliIdentity struct {
	Version           string   `json:"version"`
	Platform          string   `json:"platform"`
	URL               string   `json:"url"`
	ArchiveSHA256     string   `json:"archiveSHA256"`
	DroppedExceptions []string `json:"droppedUpstreamTestExceptions,omitempty"`
}

// moveCLI rewrites tools.treeSitterCLI and the CLI archive table of the pins map m for this platform only (the committed
// archives of other platforms belong to the committed version) and drops each grammar's upstreamTestException.
func moveCLI(m map[string]any, version, platform, archive string) (*cliIdentity, error) {
	if !cliVersionRx.MatchString(version) {
		return nil, fmt.Errorf("-cli-version %q is not X.Y.Z", version)
	}
	tools := stringMap(m, "tools")
	committed, _ := stringMap(stringMap(tools, "treeSitterCLIArchives"), platform)["url"].(string)
	if committed == "" {
		return nil, fmt.Errorf("no CLI archive is pinned for %s", platform)
	}
	if base := path.Base(committed); filepath.Base(archive) != base {
		return nil, fmt.Errorf("-cli-archive is named %s, the pinned asset for %s is %s", filepath.Base(archive), platform, base)
	}
	sum, err := sumFile(archive)
	if err != nil {
		return nil, err
	}
	id := &cliIdentity{Version: version, Platform: platform, URL: cliAssetBase + "v" + version + "/" + path.Base(committed), ArchiveSHA256: sum}
	tools["treeSitterCLI"] = version
	tools["treeSitterCLIArchives"] = map[string]any{platform: map[string]any{"url": id.URL, "sha256": sum}}
	for name, g := range stringMap(m, "grammars") {
		if gm, _ := g.(map[string]any); gm != nil && gm["upstreamTestException"] != nil {
			delete(gm, "upstreamTestException")
			id.DroppedExceptions = append(id.DroppedExceptions, name)
		}
	}
	sort.Strings(id.DroppedExceptions)
	return id, nil
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
	cliVersion := fs.String("cli-version", "", "also move the CLI: X.Y.Z of the candidate release (needs -cli-archive); the copy then needs gen -cli-sandbox")
	cliArchive := fs.String("cli-archive", "", "downloaded candidate CLI archive of this host's platform (file name as the pinned asset)")
	cliPlatform := fs.String("cli-platform", hostPlatform(), "platform key of -cli-archive")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *src == "" || *archive == "" || *out == "" {
		return errors.New("-src, -archive and -out are required")
	}
	if (*cliVersion == "") != (*cliArchive == "") {
		return errors.New("-cli-version and -cli-archive go together")
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
	var cli *cliIdentity
	if *cliVersion != "" {
		if cli, err = moveCLI(m, *cliVersion, *cliPlatform, *cliArchive); err != nil {
			return err
		}
	}
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
	return json.NewEncoder(stdout).Encode(struct {
		RuntimePin
		CLI *cliIdentity `json:"cli,omitempty"`
	}{rt, cli})
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
