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
	if ok {
		for _, dep := range info.Deps {
			if dep.Path != ids.Baseline.Module {
				continue
			}
			if dep.Version != ids.Baseline.Version || dep.Replace != nil {
				fmt.Printf("identity mismatch: resolved %s, expected %s, replacement=%t\n", dep.Version, ids.Baseline.Version, dep.Replace != nil)
				os.Exit(1)
			}
			fmt.Printf("%s %s\n", dep.Path, dep.Version)
			return
		}
	}
	fmt.Println("baseline dependency missing from build information")
	os.Exit(1)
}
