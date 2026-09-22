Large internal recovery traces, the reduction matrix, full intermediate tree
dumps and the rejected C# patch are retained locally in this same phase and
excluded from Git. They are diagnostic investigation data; their filenames
and identities remain referenced by the retained summaries and command logs.
The canonical fixtures, exact known signatures and complete C records required
by all product gates stay tracked under testdata/oracle. No product test needs
the ignored internal trace files. Local absence of those files would remove
the detailed scheduling trace evidence, not authorize reconstructing it from
a different phase or claiming the C# investigation was complete.
