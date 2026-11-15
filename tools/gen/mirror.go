package main

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// mirrorAssets lists the union of host plans; shared source archives appear once.
func (p *Pins) mirrorAssets() ([]string, error) {
	files := map[string]string{}
	add := func(file, sum, upstream string) error {
		line := strings.Join([]string{file, sum, upstream}, "\t")
		if old, ok := files[file]; ok && old != line {
			return fmt.Errorf("conflicting release asset %s", file)
		}
		files[file] = line
		return nil
	}
	for platform := range p.Tools.WasiSDKArchives {
		lines, err := p.plan(platform)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			f := strings.Split(line, "\t")
			if f[0] == "goinstall" {
				continue
			}
			if err := add(f[1], f[3], f[2]); err != nil {
				return nil, err
			}
		}
	}
	for _, archive := range p.Tools.TreeSitterCLIArchives {
		if err := add(path.Base(archive.URL), archive.SHA256, archive.URL); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, files[name])
	}
	return out, nil
}

func validateMirrors(bases []string) error {
	for _, base := range bases {
		u, err := url.Parse(base)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(base, ",\t\r\n ") {
			return fmt.Errorf("invalid HTTPS mirror base %q", base)
		}
	}
	return nil
}
