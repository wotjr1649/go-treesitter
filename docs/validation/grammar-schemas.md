# Grammar node-type schemas

Register of the static node-type schema (`node-types.json`) of every pinned grammar, as ADR-0020 decision 2 requires
with each grammar change. The file is read from the pinned upstream archive after its sha256 is verified against
`tools/gen/pins.json`; it is not generated or modified by this repository. A schema hash says nothing about parse
behaviour: equal schemas can still parse differently (for example after a precedence change).

A new grammar's first entry is its baseline. When a pin moves, the entry records the old and new hashes and the
reviewed node-type diff (tooling: `docs/design/decisions/ADR-0025` once accepted; until then the diff is written by
hand in the pin-change pull request).

## v0.3.0 (root tag on the merge commit of this change)

Pins as on `main` at `a44ea38`. `tsx` and `typescript` come from one archive.

| Grammar | Repository | Commit | Directory | Bytes | `node-types.json` SHA-256 | Status |
|---|---|---|---|---:|---|---|
| bash | tree-sitter/tree-sitter-bash | `a06c2e4415e9` | `src` | 51167 | `b644629f7f9460cb0af080cb88f3bef898e6c47b6f8a76ed512b67ae16ef5ac5` | baseline (new in v0.3.0) |
| c | tree-sitter/tree-sitter-c | `b780e47fc780` | `src` | 83057 | `23e819ef1eefd357bb6eba844f47f8492f6a88da3e5046b06ec4acd4da8a9fd6` | baseline (new in v0.3.0) |
| cpp | tree-sitter/tree-sitter-cpp | `c00922280863` | `src` | 164416 | `bc49c50d6b8af62df4cfda2675e5142bfba85bc1790ff2693aa0ca1286d5c7fe` | baseline (new in v0.3.0) |
| csharp | tree-sitter/tree-sitter-c-sharp | `9150f7d56bb4` | `src` | 133257 | `54bf9fa925bac6fcdc05e0221ae8d4a62c155eb43ccd72b5bc2a4bdbebb7997e` | pin unchanged since v0.2.0; first recorded here |
| css | tree-sitter/tree-sitter-css | `dda5cfc5722c` | `src` | 43620 | `fbbf8d22d82608f6d2f61cfdb374e6758779754fb2f42643e9dc5af6ead0d5d2` | baseline (new in v0.3.0) |
| golang | tree-sitter/tree-sitter-go | `2346a3ab1bb3` | `src` | 52032 | `8d77e723df0f0dfccb66d4571a5c3c17ecfb6c90959044c0992146b21f383bff` | pin unchanged since v0.2.0; first recorded here |
| html | tree-sitter/tree-sitter-html | `73a3947324f6` | `src` | 5195 | `e04d0a6e667ee3773ceeb0751466ce314f29bafc9b1c7d2466414c39c3eda178` | baseline (new in v0.3.0) |
| java | tree-sitter/tree-sitter-java | `e10607b45ff7` | `src` | 81249 | `19c46facc653381c337ff6cad75dd8b052524179a366c80825d6d0010520eef2` | baseline (new in v0.3.0) |
| javascript | tree-sitter/tree-sitter-javascript | `58404d8cf191` | `src` | 64022 | `188a8baa97018edaf63bf7ece372c07bc0d74d1301004bc8f211eb7dee5a6e70` | baseline (new in v0.3.0) |
| json | tree-sitter/tree-sitter-json | `254c42a64764` | `src` | 2698 | `620b6f5d38676a97526e9d128972d4b156ddbdf6e130cd7e6b9c35e6823e943f` | pin unchanged since v0.2.0; first recorded here |
| python | tree-sitter/tree-sitter-python | `26855eabccb1` | `src` | 65198 | `6d26089d798b66b9a68dd5f6e278cf1ea00b31537ac48a7efbbe5bb63094c473` | pin unchanged since v0.2.0; first recorded here |
| rust | tree-sitter/tree-sitter-rust | `77a3747266f4` | `src` | 99023 | `4b73a1248978340336100db455bf0731c23f9190568c9ae62265fa4a80a327d5` | baseline (new in v0.3.0) |
| tsql | meloncholera/tree-sitter-mssql | `8620fbcfca94` | `src` | 404735 | `d49356aed210b6522a6e130fdfb3081ff004b74c1cc3a8aee8c5f76eb73d30b9` | pin unchanged since v0.2.0; first recorded here |
| tsx | tree-sitter/tree-sitter-typescript | `75b3874edb2d` | `tsx/src` | 113345 | `78b5789145286799a27a0a7ecc36cc1bcb151f94ec7fa631b248459867010c8c` | pin unchanged since v0.2.0; first recorded here |
| typescript | tree-sitter/tree-sitter-typescript | `75b3874edb2d` | `typescript/src` | 108583 | `c790a733fc756b54d4e54dceeb7d2d51e40d8b57136e70277753a75804cce3e3` | pin unchanged since v0.2.0; first recorded here |

## v0.4.0 candidate (wave-2 batch 2)

First schema baselines for the eight new grammar pins in the wave-2 batch 2 pull request, based on main `2a1d01d` (engine ABI 3).
The 15 v0.3.0 records above remain unchanged. These are schema identities, not release or parse-behaviour claims.
YAML's ADR-0023 patch changes `src/parser.c` only; the schema below is the unmodified pinned upstream file.

| Grammar | Repository | Commit | Directory | Bytes | `node-types.json` SHA-256 | Status |
|---|---|---|---|---:|---|---|
| dart | UserNobody14/tree-sitter-dart | `be07cf7118d3` | `src` | 314023 | `ea858f11158d4b176b32e7fdbdff6b49c587b2e7ee70e5fbe90751865a1912fe` | baseline (Q14 v0.4.0 candidate) |
| kotlin | fwcd/tree-sitter-kotlin | `d6af96d2e23e` | `src` | 211132 | `c52aeae846963fe9bc82cc81e2715344e9976fff99469a127c91988ad3787c72` | baseline (Q14 v0.4.0 candidate) |
| php | tree-sitter/tree-sitter-php | `92b5271b60be` | `php/src` | 115690 | `c487d95cf9ddce3b7fe2120a257e5e43f7e9e4842cb5988707a984887143b788` | baseline (Q14 v0.4.0 candidate) |
| powershell | airbus-cert/tree-sitter-powershell | `e7bd348c49fd` | `src` | 72248 | `dc314fa2746de2c2048ad5c95e0c6488d1b2873e524a95473356c685f18a9707` | baseline (Q14 v0.4.0 candidate) |
| r | r-lib/tree-sitter-r | `58a22794466c` | `src` | 69859 | `94d900768d13cfc5daa8eefa829c297c1bc0c09582574f381f9ff8a6c5103062` | baseline (Q14 v0.4.0 candidate) |
| ruby | tree-sitter/tree-sitter-ruby | `ad907a69da0c` | `src` | 70025 | `9683f80e7be3dd93f1d6f575800de3f3445cda8af1f313f1f80003229c328e8b` | baseline (Q14 v0.4.0 candidate) |
| xml | tree-sitter-grammars/tree-sitter-xml | `5000ae8f22d1` | `xml/src` | 17444 | `3b63e111187a0f9b2282e1748db2dd6b10e968f0cdb3676772def93bf89f2a7b` | baseline (Q14 v0.4.0 candidate) |
| yaml | tree-sitter-grammars/tree-sitter-yaml | `a1c4812a73ec` | `src` | 9026 | `1238b6c89fc6afb9723b6463a365843aa87e478025490660e5acfc9adaaf809b` | baseline (Q14 v0.4.0 candidate) |
