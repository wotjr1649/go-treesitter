// Command native-ref builds the native C reference executables (one per grammar) from the sources pinned in
// tools/gen/pins.json: the unmodified tree-sitter runtime plus one unmodified grammar, linked with native_ref.c.
// They produce the dumps and changed ranges the CGO-free conformance lane compares against (docs/specs/oracle.md).
//
//	go run ./tools/native-ref -dl <archive dir> -work <empty extract dir> -out <exe dir> [-fetch] [lang ...]
//
// Every archive's sha256 is verified against the pin before it is extracted, and every grammar source file against
// its pinned "files" hash. Nothing inside an archive is executed; only the C compiler reads it.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type source struct {
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"`
	ExtractDir    string            `json:"extractDir"`
	Archive       string            `json:"archive"`
	ArchiveURL    string            `json:"archiveURL"`
	ArchiveSHA256 string            `json:"archiveSHA256"`
	GrammarDir    string            `json:"grammarDir"`
	Dirs          []string          `json:"dirs"`
	Files         map[string]string `json:"files"`
	CSymbol       string            `json:"cSymbol"`
}

type pins struct {
	Runtime  source            `json:"runtime"`
	WasiLibc source            `json:"wasiLibc"`
	Grammars map[string]source `json:"grammars"`
}

func main() {
	log := flag.String("pins", filepath.Join("tools", "gen", "pins.json"), "pins file")
	dl := flag.String("dl", "", "directory holding (or receiving, with -fetch) the pinned archives")
	work := flag.String("work", "", "directory the archives are extracted into (must be empty or absent)")
	out := flag.String("out", "", "directory receiving <lang>[.exe]")
	cc := flag.String("cc", "gcc", "C compiler (gcc or clang)")
	opt := flag.String("opt", "-O2", "optimization flag")
	fetch := flag.Bool("fetch", false, "download a missing archive from GitHub (sha256 still verified)")
	corpus := flag.String("corpus", "", "also extract each grammar archive's test/corpus into DIR/<extractDir>/test/corpus (the archive is sha256-verified first)")
	queries := flag.String("queries", "", "also extract each grammar archive's queries/ into DIR/<extractDir>/queries (the archive is sha256-verified first)")
	nobuild := flag.Bool("nobuild", false, "verify and extract only; do not compile (-out is then optional)")
	flag.Parse()
	if *dl == "" || *work == "" || (*out == "" && !*nobuild) {
		fmt.Fprintln(os.Stderr, "usage: native-ref -dl DIR -work DIR -out DIR [-pins F] [-cc gcc] [-fetch] [-corpus DIR] [-queries DIR] [-nobuild] [lang ...]")
		os.Exit(2)
	}
	if err := run(*log, *dl, *work, *out, *cc, *opt, *corpus, *queries, *fetch, *nobuild, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "native-ref:", err)
		os.Exit(1)
	}
}

func run(pinsFile, dl, work, out, cc, opt, corpus, queries string, fetch, nobuild bool, langs []string) error {
	raw, err := os.ReadFile(pinsFile)
	if err != nil {
		return err
	}
	var p pins
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if len(langs) == 0 {
		for l := range p.Grammars {
			langs = append(langs, l)
		}
		sort.Strings(langs)
	}
	if ents, err := os.ReadDir(work); err == nil && len(ents) > 0 {
		return fmt.Errorf("%s is not empty", work)
	}
	for _, d := range []string{dl, work, out} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	extracted := map[string]bool{} // typescript and tsx share one archive
	corpusDone, queriesDone := map[string]bool{}, map[string]bool{}
	if err := prepare(p.Runtime, dl, work, fetch, extracted); err != nil {
		return fmt.Errorf("runtime: %w", err)
	}
	here, err := sourceDir()
	if err != nil {
		return err
	}
	var ctypeObjects []string
	if !nobuild {
		ctypeObjects, err = buildCtype(p.WasiLibc, dl, work, here, cc, opt, fetch)
		if err != nil {
			return fmt.Errorf("wasi-libc ctype: %w", err)
		}
	}
	for _, l := range langs {
		switch l {
		case "go":
			l = "golang" // the corpus calls it go, pins.json golang
		case "jsx":
			l = "javascript" // JSX uses the JavaScript parser
		}
		g, ok := p.Grammars[l]
		if !ok {
			return fmt.Errorf("no grammar %q in %s", l, pinsFile)
		}
		if err := prepare(g, dl, work, fetch, extracted); err != nil {
			return fmt.Errorf("%s: %w", l, err)
		}
		gdir := filepath.Join(work, g.ExtractDir, filepath.FromSlash(g.GrammarDir))
		for name, want := range g.Files {
			if got, err := fileSHA(filepath.Join(gdir, name)); err != nil || got != want {
				return fmt.Errorf("%s: %s sha256 %s, pinned %s (%v)", l, name, got, want, err)
			}
		}
		if corpus != "" && !corpusDone[g.Archive] { // prepare has verified the archive's sha256
			if err := extract(filepath.Join(dl, g.Archive), filepath.Join(corpus, g.ExtractDir), []string{"test/corpus"}); err != nil {
				return fmt.Errorf("%s: corpus: %w", l, err)
			}
			corpusDone[g.Archive] = true
		}
		if queries != "" && !queriesDone[g.Archive] { // likewise verified; typescript and tsx share one archive
			if err := extract(filepath.Join(dl, g.Archive), filepath.Join(queries, g.ExtractDir), []string{"queries"}); err != nil {
				return fmt.Errorf("%s: queries: %w", l, err)
			}
			queriesDone[g.Archive] = true
		}
		if nobuild {
			continue
		}
		lib := filepath.Join(work, p.Runtime.ExtractDir, "lib")
		exe := filepath.Join(out, l)
		if runtime.GOOS == "windows" {
			exe += ".exe"
		}
		args := []string{"-std=c11", opt, "-I" + filepath.Join(here, "shim"), "-I" + filepath.Join(lib, "include"), "-I" + filepath.Join(lib, "src"),
			"-I" + gdir, "-DTSG_LANG_FN=" + g.CSymbol}
		if runtime.GOOS == "windows" {
			args = append(args, "-static") // runs without the MinGW runtime DLLs next to it
		} else {
			args = append(args, "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE") // upstream's own Makefile flags
			if runtime.GOOS == "darwin" {
				args = append(args, "-D_DARWIN_C_SOURCE") // likewise: without it _POSIX_C_SOURCE hides macOS extensions
			}
		}
		args = append(args, filepath.Join(lib, "src", "lib.c"), filepath.Join(gdir, "parser.c"))
		if _, ok := g.Files["scanner.c"]; ok {
			args = append(args, filepath.Join(gdir, "scanner.c"))
		}
		args = append(args, ctypeObjects...)
		args = append(args, filepath.Join(here, "native_ref.c"), "-o", exe)
		fmt.Println(cc, strings.Join(args, " "))
		cmd := exec.Command(cc, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: compile: %w", l, err)
		}
		sum, _ := fileSHA(exe)
		fmt.Printf("built %s sha256=%s\n", exe, sum)
	}
	return nil
}

// sourceDir is the directory of native_ref.c (a subdirectory because a Go package directory may not hold C files with CGO off).
func sourceDir() (string, error) {
	for _, d := range []string{filepath.Join("tools", "native-ref", "c"), "c"} {
		if _, err := os.Stat(filepath.Join(d, "native_ref.c")); err == nil {
			return filepath.Abs(d)
		}
	}
	return "", errors.New("c/native_ref.c not found; run from the repository root or tools/native-ref")
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}

// prepare verifies the source's archive and extracts the pinned dirs into <work>/<extractDir>, once per archive.
func prepare(s source, dl, work string, fetch bool, done map[string]bool) error {
	key := s.Archive + "|" + strings.Join(s.Dirs, ",") // typescript and tsx share an archive but extract different dirs
	if done[key] {
		return nil
	}
	arc := filepath.Join(dl, s.Archive)
	if _, err := os.Stat(arc); errors.Is(err, os.ErrNotExist) && fetch {
		const gh = "https://github.com/"
		if !strings.HasPrefix(s.Repo, gh) || len(s.Commit) != 40 {
			return fmt.Errorf("cannot derive a tarball url from %q @ %q", s.Repo, s.Commit)
		}
		url := s.ArchiveURL
		if url == "" {
			url = "https://api.github.com/repos/" + strings.TrimPrefix(s.Repo, gh) + "/tarball/" + s.Commit
		}
		if err := download(url, arc); err != nil {
			return err
		}
	}
	if got, err := fileSHA(arc); err != nil || got != s.ArchiveSHA256 {
		return fmt.Errorf("archive %s sha256 %s, pinned %s (%v)", arc, got, s.ArchiveSHA256, err)
	}
	if err := extract(arc, filepath.Join(work, s.ExtractDir), s.Dirs); err != nil {
		return err
	}
	done[key] = true
	return nil
}

// extract writes the regular files under dirs (top directory of the archive stripped) into dst. Entries that would
// leave dst are an error; links and special files are skipped.
func extract(arc, dst string, dirs []string) error {
	f, err := os.Open(arc)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(path.Clean(h.Name), "/")
		if !ok || h.Typeflag != tar.TypeReg {
			continue
		}
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("archive entry %q escapes the target", h.Name)
		}
		keep := false
		for _, d := range dirs {
			keep = keep || strings.HasPrefix(rel, d+"/")
		}
		if !keep {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		w, err := os.Create(p)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, tr); err != nil {
			w.Close()
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
	}
}
