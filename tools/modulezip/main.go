// modulezip uses the same x/mod revision vendored by Go 1.27.2 (unchanged from 1.27.1).
package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

func main() {
	version := flag.String("version", "v0.0.1", "version used for the module ZIP entry prefix")
	flag.Parse()
	if flag.NArg() != 5 {
		panic("modulezip [-version VERSION] MODULE ROOT REVISION SUBDIR OUTPUT")
	}
	m := module.Version{Path: flag.Arg(0), Version: *version}
	if err := module.Check(m.Path, m.Version); err != nil {
		panic(err)
	}
	path := flag.Arg(4)
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	err = zip.CreateFromVCS(output, m, flag.Arg(1), flag.Arg(2), flag.Arg(3))
	closeErr := output.Close()
	if err != nil {
		panic(err)
	}
	if closeErr != nil {
		panic(closeErr)
	}
	checked, err := zip.CheckZip(m, path)
	if err != nil {
		panic(err)
	}
	fmt.Printf("module ZIP %s: %d valid entries\n", m.Path, len(checked.Valid))
}
