// modulezip uses the same x/mod revision vendored by Go 1.27.1.
package main

import (
	"fmt"
	"os"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

func main() {
	if len(os.Args) != 6 {
		panic("modulezip MODULE ROOT REVISION SUBDIR OUTPUT")
	}
	output, err := os.OpenFile(os.Args[5], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	m := module.Version{Path: os.Args[1], Version: "v0.0.1"}
	err = zip.CreateFromVCS(output, m, os.Args[2], os.Args[3], os.Args[4])
	closeErr := output.Close()
	if err != nil {
		panic(err)
	}
	if closeErr != nil {
		panic(closeErr)
	}
	checked, err := zip.CheckZip(m, os.Args[5])
	if err != nil {
		panic(err)
	}
	fmt.Printf("module ZIP %s: %d valid entries\n", m.Path, len(checked.Valid))
}
