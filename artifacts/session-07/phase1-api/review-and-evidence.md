# Public consumer entry point

Adds treesitter.New returning the existing syntax.Parser. The adapter stays
internal and no upstream type crosses the public signature. Baseline, grammar
blobs, oracle epoch and known-regression behavior are unchanged.

The retained TestExternalConsumer writes a separate module and runs the public
API with CGO disabled. It checks a fresh parse, rejected invalid edit, a valid
length-changing multiline edit, edited/fresh snapshot equality, old-tree
release, an explicit usable error tree and cancellation. A separate source in
that external module must fail compilation when it imports the internal adapter.
The comparison does not require reuse: the documented full-reparse fallback is
allowed. ExampleNew is executable documentation.

Independent api_review identified three test gaps: ignoring edits could go
unnoticed, no negative internal-import check, and an unnecessary double Close.
All three were addressed before focused-02 and regression-02. The invalid edit
now makes an implementation that ignores Previous/Edit fail. The valid edit
changes both byte length and line points. Each tree is closed once.

Observed checks in this phase (Windows/amd64, CGO_ENABLED=0, Go 1.27.1):

- focused-01: initial external consumer and import boundary passed.
- regression-01: all five packages passed; vet-01 passed.
- focused-02: strengthened consumer, negative import and example passed.
- regression-02: all five packages passed, including current oracle/ratchets.
- git diff --check: passed for the implementation/doc working diff.

commands.jsonl binds commands and unchanged module/identity file hashes.
No fixture changes, runtime patch, remote action or merge occurred. The empty
vet command emitted no text; its log contains the initial wrapper's LF only.

Evidence level E3. This closes public construction/integration mechanics, not
the open language correctness or full release gates. NOT_RUN in this phase:
ARM64 execution, remote CI, race diagnostics, broader corpus and resource tests.
