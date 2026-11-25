package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

const (
	maxFile  = 20 << 20
	maxTotal = 25 << 20
)

// allowed lists the file names a sandbox may leave in its result volume. Anything else (links, binaries, module
// caches, environment dumps, unexpected paths) is rejected, not copied.
var allowed = regexp.MustCompile("^(measurements\\.jsonl|cells\\.json|run\\.json|correctness\\.csv|builds\\.csv|workload\\.json|fixtures-(time|alloc)\\.csv|summary\\.md|provenance/[A-Za-z0-9._-]{1,80}\\.(json|txt|csv)|failures/[A-Za-z0-9._-]{1,80}\\.txt|go-sum-proposal/[a-z]{3,20}/go\\.(mod|sum))$")

func cmdCollect(args []string) error {
	fs := flag.NewFlagSet("collect", flag.ExitOnError)
	in := fs.String("in", "", "untrusted result directory")
	out := fs.String("out", "", "directory for the accepted files")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return fmt.Errorf("collect needs -in and -out")
	}
	var accepted []string
	var rejected []string
	var total int64
	var oversize []string
	err := filepath.WalkDir(*in, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(*in, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		info, err := os.Lstat(p)
		switch {
		case err != nil:
			return err
		case !info.Mode().IsRegular():
			rejected = append(rejected, rel+": not a regular file")
		case !allowed.MatchString(rel):
			rejected = append(rejected, rel+": unexpected path")
		case info.Size() > maxFile || total+info.Size() > maxTotal:
			b, _ := os.ReadFile(p) // hash only: the status stays reproducible although the content is not kept
			h := sha256.Sum256(b)
			oversize = append(oversize, fmt.Sprintf("%s %d %s", rel, info.Size(), hex.EncodeToString(h[:])))
		default:
			if verr := validate(p, rel); verr != nil {
				rejected = append(rejected, rel+": "+verr.Error())
			} else {
				total += info.Size()
				accepted = append(accepted, rel)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(accepted)
	var sums strings.Builder
	for _, rel := range accepted {
		b, err := os.ReadFile(filepath.Join(*in, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if err := writeFileAll(filepath.Join(*out, filepath.FromSlash(rel)), b); err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%s  %s\n", sha(b), rel)
	}
	if err := writeFileAll(filepath.Join(*out, "SHA256SUMS"), []byte(sums.String())); err != nil {
		return err
	}
	if len(oversize) > 0 {
		if err := writeFileAll(filepath.Join(*out, "OVERSIZE.txt"), []byte("files over the upload budget (name size sha256); the run stays incomplete:\n"+strings.Join(oversize, "\n")+"\n")); err != nil {
			return err
		}
	}
	if len(rejected) > 0 || len(oversize) > 0 {
		sort.Strings(rejected)
		_ = writeFileAll(filepath.Join(*out, "REJECTED.txt"), []byte(strings.Join(rejected, "\n")+"\n"))
		return fmt.Errorf("%d rejected, %d oversize: %s", len(rejected), len(oversize), strings.Join(append(rejected, oversize...), "; "))
	}
	return nil
}

// validate checks a file's content against its schema with bounded field sizes.
func validate(p, rel string) error {
	switch {
	case rel == "measurements.jsonl":
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(io.LimitReader(f, maxFile))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			var r Row
			dec := json.NewDecoder(strings.NewReader(sc.Text()))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&r); err != nil {
				return fmt.Errorf("line %d: %v", n, err)
			}
			if len(r.Cell) > 512 || len(r.Err) > 2048 || r.Res != nil && (r.Res.Schema != harness.Schema || len(r.Res.SamplesNs) > 1000 || len(r.Res.Error) > 1024) {
				return fmt.Errorf("line %d: field out of bounds", n)
			}
		}
		return sc.Err()
	case strings.HasSuffix(rel, ".json"):
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var v any
		return json.Unmarshal(b, &v)
	case strings.HasSuffix(rel, ".csv"):
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = csv.NewReader(f).ReadAll()
		return err
	}
	return nil
}
