# Unified diff artifact review

The staged whitespace check reported twelve space-before-tab lines inside
candidate.diff. They are unchanged context lines in a valid unified diff: its
required leading context space precedes the original Go indentation tabs.
The artifact is preserved exactly. The actual Go source and other changed files
pass their whitespace check; no whitespace setting or analyzer was weakened.
This is a data-format observation, not an unresolved scanner-source defect.
