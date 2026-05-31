# Patch Survival Analysis

Patch survival connects model spend to durable code shipped without uploading source code.

The local analyzer can classify unified-diff hunks against the current worktree or a selected git ref:

- `survived`: every added non-empty line is still present in the target file.
- `modified`: some added lines remain, but not all.
- `reverted`: none of the added lines remain.
- `unknown`: the target is unavailable or the hunk has no added lines.

Default output is upload-safe: file paths are represented as SHA-256 hashes and only counts, hunk totals, line totals, survival buckets, and optional yield ratios are emitted. Local debugging may opt in to raw paths with `ExposePaths`, but code hunks and file contents are never included in the result.

When spend data is available, callers can provide total token count and model cost to compute survived-lines-per-1k-tokens and survived-lines-per-dollar. The CLI exposes this through:

```sh
agent-analyzer patch-survival --repo /path/to/repo --diff patch.diff --tokens 120000 --cost-usd 2.40 --out patch-survival.json
```

This is the first local-only building block for paid patch-yield reporting. It supports comparing against `HEAD` or another selected ref so future scan jobs can report survived, modified, reverted, unknown, and yield aggregates while keeping cloud reports aggregate-only.
