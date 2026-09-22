package main

import (
	"fmt"
	"os"
	"runtime/debug"

	_ "github.com/wotjr1649/go-treesitter/internal/gtsadapter"
	"github.com/wotjr1649/go-treesitter/internal/provenance"
)

func main() {
	ids, err := provenance.Read()
	if err != nil {
		panic("invalid embedded identities")
	}
	info, ok := debug.ReadBuildInfo()
	if ok && len(ids.Runtime.ManifestSHA256) == 64 {
		for _, dep := range info.Deps {
			if dep.Path == ids.Baseline.Module || dep.Replace != nil {
				fmt.Printf("unexpected external runtime or replacement: %s\n", dep.Path)
				os.Exit(1)
			}
		}
		fmt.Printf("bundled %s origin=%s %s manifest=%s\n", ids.Runtime.Module, ids.Baseline.Module, ids.Baseline.Version, ids.Runtime.ManifestSHA256)
		return
	}
	fmt.Println("runtime identity or build information missing")
	os.Exit(1)
}
