# Optional GPL grammars

This separate `v0.0.1` module adds the original pinned `caddy`, `disassembly`
and `jq` grammars. Together with the base module's 203 grammars, it retains the
206-language catalog. Import it explicitly:

```go
import (
    treesitter "github.com/wotjr1649/go-treesitter"
    _ "github.com/wotjr1649/go-treesitter/grammars/gpl"
)

parser := treesitter.New()
```

The module requires the matching base `v0.0.1`; consumers need no `replace`
directive. The first version is planned, not published. Both module ZIPs have
been consumed from an empty local cache in the retained external-consumer test.

The root [LICENSE](LICENSE) contains GPL version 3. Original component notices
are in `LICENSES/`; jq explicitly declares GPL-3.0-or-later. Including these
grammars carries their GPL terms. This module is not an MIT-only extension.
The base module neither imports this module nor includes its source in its ZIP.

Registration runs during Go package initialization. Runtime types stay inside
this module's `internal/gtsadapter`; the public parser and result API remains
the base module's own types. The existing runtime cache supplies lazy grammar
loading. No network access or C runtime is used when parsing.

`sources/` contains complete pinned grammar repository archives and the source
of the upstream `ts2go` conversion tool, including its pinned Go build dependency.
See [source reproduction](sources/README.md) for the distinct artifact and
generator identities. These sources retain their original notices.
