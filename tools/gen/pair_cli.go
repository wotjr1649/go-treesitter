package main

// Runtime + CLI pair rehearsal (TEST ONLY): the CLI of the pins copy is a candidate release asset that no pin vouches for.
// The repository's one path for such a CLI is grammarpatch's Unpinned mode behind tools/gen/nightly-sandbox.sh (bubblewrap:
// no network, no token, no environment, read-only input, one writable output). This file selects that path; it adds none.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
)

const testOnlyRuntimeCLIRehearsal = "runtime-cli-rehearsal"

// rehearsalKind names the test-only mode recorded in provenance and identity.
func rehearsalKind(o opts) string {
	if o.cliMoved {
		return testOnlyRuntimeCLIRehearsal
	}
	return testOnlyRuntimeRehearsal
}

// sandboxCandidateCLI turns c into the unpinned, sandboxed configuration. Before anything runs, trusted code checks that the
// binary is the decompressed archive whose sha256 the pins copy recorded; the first execution is then `--version` inside the
// sandbox, which must report the copy's version.
func sandboxCandidateCLI(c *grammarpatch.Config, o opts, cli Archive) error {
	if o.treeSitter == "" || o.cliSandbox == "" {
		return fmt.Errorf("a candidate CLI needs -tree-sitter and -cli-sandbox")
	}
	bin, err := filepath.Abs(o.treeSitter)
	if err != nil {
		return err
	}
	wrapper, err := filepath.Abs(o.cliSandbox)
	if err != nil {
		return err
	}
	if err := grammarpatch.CheckCLIBytes(filepath.Join(o.dl, path.Base(cli.URL)), cli.SHA256, bin); err != nil {
		return fmt.Errorf("candidate CLI: %w", err)
	}
	os.Setenv("NIGHTLY_BIN", bin) // read by the wrapper
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := proc.CommandContext(ctx, wrapper, "--version")
	cmd.Env = grammarpatch.UnpinnedEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("candidate CLI --version in the sandbox: %w: %s", err, out)
	}
	if f := strings.Fields(string(out)); len(f) < 2 || f[0] != "tree-sitter" || f[1] != c.CLIVersion {
		return fmt.Errorf("candidate CLI reports %q, the pins copy says %s", strings.TrimSpace(string(out)), c.CLIVersion)
	}
	c.CLI, c.Unpinned, c.CLIArchive, c.CLIArchiveSHA256 = wrapper, true, "", ""
	return nil
}
