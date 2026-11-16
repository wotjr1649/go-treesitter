package grammarpatch

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type NPMInput struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Resolved        string `json:"resolved"`
	Integrity       string `json:"integrity"`
	LockfileSHA256  string `json:"lockfileSHA256"`
	ArchiveSHA256   string `json:"archiveSHA256"`
	ScriptsExecuted bool   `json:"scriptsExecuted"`
}

func checkIntegrity(b []byte, integrity string) error {
	if !strings.HasPrefix(integrity, "sha512-") {
		return fmt.Errorf("npm requires a sha512 integrity hash: %q", integrity)
	}
	sum := sha512.Sum512(b)
	if "sha512-"+base64.StdEncoding.EncodeToString(sum[:]) != integrity {
		return fmt.Errorf("npm integrity mismatch")
	}
	return nil
}

// Only selected grammar inputs are unpacked; npm and lifecycle scripts never run.
func prepareNPM(c Config, root string) ([]NPMInput, error) {
	if len(c.NPMDependencies) == 0 {
		return nil, nil
	}
	if !c.Regenerate {
		return nil, fmt.Errorf("npm grammar inputs require regeneration")
	}
	lock, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil {
		return nil, err
	}
	var data struct {
		Version  int                                                      `json:"lockfileVersion"`
		Packages map[string]struct{ Version, Resolved, Integrity string } `json:"packages"`
	}
	if err := json.Unmarshal(lock, &data); err != nil {
		return nil, err
	}
	if data.Version < 2 || data.Version > 3 {
		return nil, fmt.Errorf("npm grammar inputs require package-lock v2 or v3")
	}
	names := append([]string(nil), c.NPMDependencies...)
	sort.Strings(names)
	var inputs []NPMInput
	for i, name := range names {
		if !local(name) || strings.Contains(name, "node_modules") || (i > 0 && names[i-1] == name) {
			return nil, fmt.Errorf("invalid or duplicate npm grammar input %q", name)
		}
		p, ok := data.Packages["node_modules/"+name]
		u, err := url.Parse(p.Resolved)
		if !ok || p.Version == "" || err != nil || u.Scheme != "https" || u.Host != "registry.npmjs.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("npm grammar input %s is not pinned to an HTTPS registry tarball", name)
		}
		if c.NPMCache == "" {
			return nil, fmt.Errorf("npm archive cache is required")
		}
		if err := os.MkdirAll(c.NPMCache, 0755); err != nil {
			return nil, err
		}
		archive := filepath.Join(c.NPMCache, hash([]byte(p.Integrity))+".tgz")
		b, err := os.ReadFile(archive)
		if os.IsNotExist(err) {
			if !c.FetchNPM {
				return nil, fmt.Errorf("npm archive missing: %s (enable fetching)", archive)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.Resolved, nil)
			if e != nil {
				return nil, e
			}
			client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return fmt.Errorf("npm tarball redirect rejected") }}
			resp, e := client.Do(req)
			if e != nil {
				return nil, e
			}
			b, e = io.ReadAll(io.LimitReader(resp.Body, (128<<20)+1))
			resp.Body.Close()
			if e != nil {
				return nil, e
			}
			if resp.StatusCode != http.StatusOK || len(b) > 128<<20 {
				return nil, fmt.Errorf("npm tarball %s: status %s or size limit", name, resp.Status)
			}
			if e := checkIntegrity(b, p.Integrity); e != nil {
				return nil, e
			}
			f, e := os.CreateTemp(c.NPMCache, "npm-*.tmp")
			if e != nil {
				return nil, e
			}
			tmp := f.Name()
			_, e = f.Write(b)
			ce := f.Close()
			if e == nil {
				e = ce
			}
			if e == nil {
				e = os.Rename(tmp, archive)
			}
			os.Remove(tmp)
			if e != nil {
				return nil, e
			}
		} else if err != nil {
			return nil, err
		}
		if err := checkIntegrity(b, p.Integrity); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		// QuickJS resolves bare imports from cwd/node_modules, including imports in ../common.
		dest := filepath.Join(root, filepath.Dir(filepath.FromSlash(c.GrammarDir)), "node_modules", filepath.FromSlash(name))
		if err := extract(archive, dest); err != nil {
			return nil, err
		}
		inputs = append(inputs, NPMInput{Name: name, Version: p.Version, Resolved: p.Resolved, Integrity: p.Integrity, LockfileSHA256: hash(lock), ArchiveSHA256: hash(b)})
	}
	return inputs, nil
}
