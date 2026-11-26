package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A result bundle is one ZIP written by pack (after the measurer validated the files) and read by unpack in the aggregate
// job, which checks sizes, entry count and paths before it extracts a single byte. The bundle is the only file a job uploads.
const (
	maxEntries  = 300
	maxPackFile = 30 << 20 // the bundle on disk
)

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	dir := fs.String("dir", "", "validated directory (the output of collect)")
	out := fs.String("out", "", "bundle file to write")
	fs.Parse(args)
	if *dir == "" || *out == "" {
		return errors.New("pack needs -dir and -out")
	}
	var names []string
	var total int64
	err := filepath.WalkDir(*dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(*dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		if rel == "SHA256SUMS" { // collect's own listing; the aggregate job computes its own
			return nil
		}
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() || !allowed.MatchString(rel) || st.Size() > maxFile {
			return fmt.Errorf("%s: not an allowed regular file", rel)
		}
		if total += st.Size(); total > maxTotal || len(names) >= maxEntries {
			return errors.New("the bundle would exceed its size or entry cap")
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, rel := range names {
		b, err := readBounded(filepath.Join(*dir, filepath.FromSlash(rel)), maxFile)
		if err != nil {
			return err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()})
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// unpackBounded extracts a bundle after checking it: a regular file under maxPackFile, at most maxEntries entries, every
// name allowlisted and unique, no directory or link entries, declared sizes within the per-file and total caps, and every
// stream read through a limit that equals its declared size. It returns the number of files written.
func unpackBounded(bundle, dst string) (int, error) {
	st, err := os.Lstat(bundle)
	if err != nil {
		return 0, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxPackFile {
		return 0, errors.New("bundle is not a regular file within the size cap")
	}
	zr, err := zip.OpenReader(bundle)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	if len(zr.File) > maxEntries {
		return 0, fmt.Errorf("bundle has %d entries, over the %d cap", len(zr.File), maxEntries)
	}
	seen := map[string]bool{}
	var total uint64
	for _, f := range zr.File {
		switch {
		case f.Mode()&os.ModeType != 0 || strings.HasSuffix(f.Name, "/"):
			return 0, fmt.Errorf("entry %q is not a regular file", printable(f.Name, 80))
		case !allowed.MatchString(f.Name) || strings.Contains(f.Name, ".."):
			return 0, fmt.Errorf("entry %q is not an allowed path", printable(f.Name, 80))
		case seen[f.Name]:
			return 0, fmt.Errorf("entry %q appears twice", f.Name)
		case f.UncompressedSize64 > maxFile:
			return 0, fmt.Errorf("entry %q declares %d bytes", f.Name, f.UncompressedSize64)
		}
		seen[f.Name] = true
		if total += f.UncompressedSize64; total > maxTotal {
			return 0, errors.New("bundle declares more than the total cap")
		}
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		rc.Close()
		if err != nil || uint64(len(b)) != f.UncompressedSize64 {
			return 0, fmt.Errorf("entry %q does not match its declared size", f.Name)
		}
		if err := writeFileAll(filepath.Join(dst, filepath.FromSlash(f.Name)), b); err != nil {
			return 0, err
		}
	}
	return len(zr.File), nil
}
