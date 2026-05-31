# Patch Survival Analysis

Patch survival connects model spend to durable code shipped without uploading source code.

The local analyzer can classify unified-diff hunks against the current worktree or a selected git ref:

- `survived`: every added non-empty line is still present in the target file.
- `modified`: some added lines remain, but not all.
- `reverted`: none of the added lines remain.
- `unknown`: the target is unavailable or the hunk has no added lines.

Default output is upload-safe: file paths are represented as SHA-256 hashes and only counts, hunk totals, and survival buckets are emitted. Local debugging may opt in to raw paths with `ExposePaths`, but code hunks and file contents are never included in the result.

This is the first local-only building block for paid patch-yield reporting. It supports comparing against `HEAD` or another selected ref so future scan jobs can report survived, modified, reverted, and unknown patch counts while keeping cloud reports aggregate-only.
