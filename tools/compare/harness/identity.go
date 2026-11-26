package harness

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// BuildIdentity reports what the binary was actually linked from: the Go toolchain, the build flags that matter and
// the version and go.sum hash of every dependency whose path starts with one of prefixes.
func BuildIdentity(prefixes ...string) map[string]string {
	id := map[string]string{"go": runtime.Version(), "goos_goarch": runtime.GOOS + "/" + runtime.GOARCH}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return id
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "CGO_ENABLED", "-trimpath", "-ldflags", "CGO_CFLAGS", "GOAMD64", "GOARM64":
			id["build."+s.Key] = s.Value
		}
	}
	for _, d := range bi.Deps {
		for _, p := range prefixes {
			if strings.HasPrefix(d.Path, p) {
				if d.Replace != nil {
					d = d.Replace
				}
				id["dep:"+d.Path] = d.Version + " " + d.Sum
				break
			}
		}
	}
	return id
}

// MergeID copies extra into id (extra wins) and returns id.
func MergeID(id, extra map[string]string) map[string]string {
	for k, v := range extra {
		id[k] = v
	}
	return id
}
