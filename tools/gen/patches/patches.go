// Package patches applies the recorded temporary grammar fixes from ADR-0023.
package patches

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Pin identifies one single-file unified diff and both versions of its upstream source.
type Pin struct {
	File                  string `json:"file"`
	SHA256                string `json:"sha256"`
	Source                string `json:"source"`
	BeforeSHA256          string `json:"beforeSHA256"`
	AfterSHA256           string `json:"afterSHA256"`
	UpstreamReport        string `json:"upstreamReport"`
	GeneratedBeforeSHA256 string `json:"generatedBeforeSHA256,omitempty"`
	GeneratedAfterSHA256  string `json:"generatedAfterSHA256,omitempty"`
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Apply runs only after the caller verifies the original pinned source. sourceRoot must
// be a private build copy: generation never modifies the verified upstream cache.
// git apply is used for unified diff semantics; hashes reject offsets or accidental changes.
func Apply(patchRoot, sourceRoot string, pins []Pin) error {
	for _, p := range pins {
		if !filepath.IsLocal(p.File) || !filepath.IsLocal(p.Source) || strings.ContainsAny(p.File+p.Source, "\\\t\r\n") || !strings.HasPrefix(p.File, "patches/") || !strings.HasSuffix(p.File, ".patch") {
			return fmt.Errorf("invalid grammar patch paths %q / %q", p.File, p.Source)
		}
		u, e := url.Parse(p.UpstreamReport)
		if e != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("patch %s: missing HTTPS upstream report", p.File)
		}
		patch, err := os.ReadFile(filepath.Join(patchRoot, filepath.FromSlash(p.File)))
		if err != nil {
			return err
		}
		if sum(patch) != p.SHA256 {
			return fmt.Errorf("patch %s sha256 mismatch", p.File)
		}
		target := filepath.Join(sourceRoot, filepath.FromSlash(p.Source))
		st, err := os.Lstat(target)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("patch source %s is not a regular file", p.Source)
		}
		before, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if sum(before) != p.BeforeSHA256 {
			return fmt.Errorf("patch %s: before sha256 mismatch", p.File)
		}
		absolute, err := filepath.Abs(filepath.Join(patchRoot, filepath.FromSlash(p.File)))
		if err != nil {
			return err
		}
		run := func(args ...string) ([]byte, error) {
			c := exec.Command("git", append([]string{"-c", "core.autocrlf=false", "-c", "core.eol=lf", "apply", "--no-index"}, args...)...)
			c.Dir = sourceRoot
			ceiling, err := filepath.Abs(sourceRoot)
			if err != nil {
				return nil, err
			}
			c.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(ceiling))
			return c.CombinedOutput()
		}
		stat, err := run("--numstat", absolute)
		fields := strings.Split(strings.TrimSpace(string(stat)), "\t")
		if err != nil || len(fields) != 3 || fields[2] != p.Source || strings.ContainsAny(fields[0]+fields[1], "\n-") {
			return fmt.Errorf("patch %s must modify only %s: %s (%v)", p.File, p.Source, stat, err)
		}
		if out, err := run("--check", absolute); err != nil {
			return fmt.Errorf("patch %s does not apply: %s (%w)", p.File, out, err)
		}
		if out, err := run(absolute); err != nil {
			return fmt.Errorf("patch %s: %s (%w)", p.File, out, err)
		}
		after, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if sum(after) != p.AfterSHA256 {
			return fmt.Errorf("patch %s: after sha256 mismatch", p.File)
		}
	}
	return nil
}

// ApplyGenerated preserves the original C identities for the reference variant.
func ApplyGenerated(patchRoot, sourceRoot string, pins []Pin) error {
	generated := append([]Pin(nil), pins...)
	for i := range generated {
		p := &generated[i]
		if (p.GeneratedBeforeSHA256 == "") != (p.GeneratedAfterSHA256 == "") {
			return fmt.Errorf("patch %s: incomplete generated C hashes", p.File)
		}
		if p.GeneratedBeforeSHA256 != "" {
			p.BeforeSHA256, p.AfterSHA256 = p.GeneratedBeforeSHA256, p.GeneratedAfterSHA256
		}
	}
	return Apply(patchRoot, sourceRoot, generated)
}
