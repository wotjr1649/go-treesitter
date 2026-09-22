`git diff --cached --check` reports 12 space-before-tab lines in candidate.diff.
They are the mandatory leading context marker of a unified diff followed by
the original Go tab indentation. The patch is retained byte-for-byte as data.
No Go source whitespace changed and all other staged files pass the check.
No Git whitespace setting or product check was disabled.
