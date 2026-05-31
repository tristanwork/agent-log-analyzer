# Token-Saving Tooling Matrix

This matrix is the paid remediation allowlist. The plugin may recommend only the tools below, only when the matching finding exists, and only after the user accepts the waiver. Installation is never automatic.

## Category Rules

| Category | Meaning | Product rule |
| --- | --- | --- |
| Input/context tokens | Prompt text, project instructions, tool schemas, file reads, tool results, and prior conversation context sent back to the model. | Retrieval and compression tools must prove they replace broader reads or output; extra schemas, indexing summaries, MCP calls, and turns can make this worse. |
| Tool-output tokens | Shell, MCP, file, and search output that becomes model input/context. | This is the strongest default target because bounded shell output and focused reads repeatedly saved tokens without lowering task quality. |
| Output tokens | Visible assistant text and tool-call JSON. | Terse prose alone is not proof of lower full-session cost. |
| Reasoning tokens | Hidden reasoning budget reported by some harnesses, such as Codex. | Do not claim reasoning-token savings unless the harness exposes and measures them. |
| Cached input tokens | Reused context billed or quota-weighted differently from fresh input. | Keep cache effects separate from live-context quality and published API estimates. |
| Telemetry only | Usage visibility, dashboards, and statuslines. | Useful for measurement; not a reducer unless the user changes behavior. |

## Default Pack

| Tool or practice | Source | Repeated result | Product decision |
| --- | --- | ---: | --- |
| Agent Analyzer workflow | `docs/benchmarks/repeated-benchmark-suite.md` | `-12,370` estimated tokens, `-12,698` tool-output tokens, `-24.0%` published API-rate cost | Always include as the core generated plugin workflow. |
| Output-budgeted commands | Built into generated skills | Part of the positive Agent Analyzer run | Always include for tool-output bloat and final verification. |
| Retrieval hygiene | Built into generated skills | Part of the positive Agent Analyzer run | Always include for repeated file reads. |
| Session hygiene and retry breaker | Built into generated skills | Part of the positive Agent Analyzer run | Always include for context pivots and retry loops. |

## Conditional Third-Party Reducers

These tools reduced published API-rate cost in 3/3 repeated runs on the noisy benchmark. They are still conditional because each targets a specific token category.

| Tool | Source | Primary category | Mean published API-rate savings | Product decision |
| --- | --- | --- | ---: | --- |
| Semble | https://github.com/MinishLab/semble | Path-limited semantic retrieval | `41.5%` | Recommend for repeated file reads when bounded local retrieval replaces broad reads. |
| context-mode | https://github.com/mksglu/context-mode | Tool-output/input-context batching | `20.4%` | Recommend for tool-output bloat or context-growth spikes; note that visible output rose on average. |
| RTK | https://github.com/rtk-ai/rtk | Explicit shell-output compression | `18.2%` | Recommend explicit commands first; global hooks require separate approval. |
| grepai | https://github.com/yoanbernabeu/grepai | Path-constrained compact retrieval | `14.5%` | Recommend only with small limits and path filters. |

## Paid Pack Install Controls

Generated paid artifacts expose these controls in `TOOL-CATALOG.json` and the `skills/tooling-setup` guidance. The catalog should be treated as an execution plan, not a command to run blindly.

| Tool class | Emit only when | Install surface | Required binary | Verify | Rollback / uninstall | Risk notes |
| --- | --- | --- | --- | --- | --- | --- |
| `context-mode` | `tool_output_bloat` or `context_growth_spikes` is present. | Claude Code plugin marketplace: `/plugin marketplace add mksglu/context-mode`, then `/plugin install context-mode@context-mode`; CLI equivalent is `claude plugin marketplace add mksglu/context-mode` and `claude plugin install context-mode@context-mode`. | None beyond Claude Code and Node runtime used by the plugin. | Restart or `/reload-plugins`, then run `/context-mode:ctx-doctor`; CLI verification can inspect `claude plugin list --json`. | Remove the Claude Code plugin, remove any manual MCP/status-line entries, then restart Claude Code. | Adds MCP tools and hooks. It should be used only to route large tool output; it must not replace normal source inspection or enforce prose style. |
| `rtk` | Shell output dominates the report and the user accepts a higher-risk hook. | Prefer explicit RTK commands first. macOS install is `brew install rtk`; other platforms must review the upstream `rtk-ai/rtk` install docs before install. | `rtk` from `github.com/rtk-ai/rtk`, not the unrelated npm package named `rtk`. | `rtk --help` and, after hook install, a small shell command whose output is visibly summarized. | Remove RTK hooks from `.claude/settings.json`, restore any `.bak` settings file created during setup, remove `@RTK.md` references, then uninstall the binary. | High install and data-movement risk because hooks can rewrite shell execution. Requires explicit waiver and per-project confirmation. |
| `semble` | Repeated file reads are present and the task has a bounded path or module target. | For MCP: `claude mcp add semble -s user -- uvx --from "semble[mcp]" semble`; for CLI-only use: `uv tool install semble` or `pip install semble`. | `semble` for CLI workflows; `uvx` for MCP workflows. | `semble --help` or an MCP list/doctor command in the target harness. Run one path-limited query before recommending it broadly. | `uv tool uninstall semble` or `pip uninstall semble`; remove the MCP server entry and clear its local index/cache if no longer needed. | Local CPU retrieval is lower data-movement risk than hosted search, but broad indexing can still add stale or irrelevant context. |
| `grepai` | Repeated grep/read loops are present and the user has a local embedding path with small search limits. | macOS: `brew install yoanbernabeu/tap/grepai`; Linux/Windows install scripts must be reviewed from the upstream repo before use. | `grepai`, plus the local embedding/runtime dependency selected by its setup. | `grepai --help`, then a small path-filtered query with capped results. | Uninstall via the same package manager, stop any watcher/daemon, and remove local indexes if the project no longer uses them. | Keep path filters and result limits mandatory. Do not run broad whole-repo semantic search as a default substitute for `rg`. |
| Official code-intelligence plugins | A matching package manager is detected in the sanitized report. | Claude Code official plugins, for example `claude plugin install pyright-lsp@claude-plugins-official`. | Matching language server: `typescript-language-server`, `pyright-langserver`, `gopls`, `rust-analyzer`, or `intelephense`. | Run the language server `--version` or equivalent, then inspect `claude plugin list --json`. | Remove the plugin and uninstall the language-server binary if it was installed only for this workflow. | Prefer project-checked setup docs over generic global installs. Do not install all language servers just because several ecosystems are mentioned in logs. |
| Official MCP/integration plugins | The sanitized report already shows a known integration such as GitHub, Notion, Linear, Sentry, or Supabase. | Claude Code official plugin install, for example `claude plugin install github@claude-plugins-official`. | Usually none, but account authentication is required inside Claude Code. | `claude plugin list --json`, then a small read-only lookup in the target integration. | Remove the plugin and revoke integration authorization if the user no longer wants it. | These can access third-party workspace data. Keep recommendations source-specific and do not infer private connector names. |
| `ccusage` | The user wants independent measurement or budget visibility. | One-shot command: `npx ccusage@latest`; no persistent install is required. | None for one-shot npm execution. | `npx ccusage@latest --help`, then a scoped daily/weekly report. | No rollback for one-shot execution; clear npm cache only if the user wants to remove cached packages. | Measurement only. It must not be described as reducing input, output, reasoning, or tool-output tokens. |

## Recommendation Thresholds

- Always include the Agent Analyzer workflow, output-budgeted commands, retrieval hygiene, and session hygiene in the generated plugin.
- Recommend third-party reducers only when the matching finding exists and the repeated benchmark has a positive result for the matching token category.
- Prefer lower-risk controls first: built-in guidance, capped shell output, path-limited `rg`, then vetted retrieval/MCP tools.
- Do not recommend hooks, shell rewriting, broad indexing, or hosted connectors without a waiver and an explicit verification command.
- Do not emit unknown private MCP, plugin, skill, repo, prompt, file-path, or host names from logs. Unknown tools remain count-only.
- If a tool source, license, binary name, install command, or rollback path cannot be verified, keep it `research_only` and out of paid-pack defaults.

## Measurement, Not Reduction

| Tool | Source | Decision |
| --- | --- | --- |
| ccusage | https://github.com/ryoppippi/ccusage | Use as independent accounting if users want it; never label as a direct token reducer. |
| ccstatusline | https://github.com/sirmalloc/ccstatusline | Use as optional awareness outside the prompt path; never label as a direct reducer. |
| Claude Code Usage Monitor / Tracker | Public usage monitor projects | Optional visibility tools; keep outside the generated paid pack unless the user asks for monitoring. |

## Removed From Default Recommendations

| Tool | Source | Repeated result | Decision |
| --- | --- | ---: | --- |
| claude-context | https://github.com/zilliztech/claude-context | `+7,327` estimated tokens, `+26.0%` API-rate cost | Do not recommend for this workflow. Revisit only with a larger retrieval-amortization benchmark. |
| Probe | https://github.com/probelabs/probe | `+874` estimated tokens, `+16.6%` API-rate cost | Do not recommend as a reducer. |
| Caveman for Claude Code | https://github.com/JuliusBrussee/caveman | `+4,355` estimated tokens, `+3.9%` API-rate cost | Keep out of Claude plugin guidance. It helped Codex in this fixture, but worsened Claude Code. |
| claude-rlm | https://github.com/Tenobrus/claude-rlm | `+19,477` estimated tokens on root+subagent aggregate | Do not recommend for medium-context tasks; root stdout cost is incomplete for sub-agent usage. |
| claude-token-efficient | https://github.com/drona23/claude-token-efficient | `1.8%` API-rate savings | Too small/noisy for the default pack; use only as manual verbosity hygiene if a user asks. |
| Squeez | https://github.com/claudioemmanuel/squeez | `-12.1%` API-rate savings in the old noisy-shell fixture | Do not recommend. It conflicts with Spec Kitty workflows, so the benchmark result stays visible only as historical evidence. |
| Broad ecosystem lists and generic plugin installs | Various | Not benchmarked as reducers | Do not ship as default remediation advice. |

## Cost Translation Rule

Dollar claims must name both the token category and the pricing surface:

- Claude Sonnet 4.6 API estimates use input, 1-hour cache-write, cache-read, and output rates.
- Codex API estimates use input, cached-input, output, and separately reported reasoning-output tokens where available.
- Native Claude Code or Codex billing can differ from direct API inference rates.
- Monthly savings are scaled linearly from the repeated benchmark percentage: `monthly savings = comparable monthly baseline spend * savings percent`.

For the core Agent Analyzer workflow, the repeated published API-rate savings were `23.986%`. At comparable workload scale, that is about `$1,199/month` on `$5,000/month` of Claude Sonnet API-equivalent coding usage, or about `$2,399/month` on `$10,000/month`.

## Guardrails

- Prefer explicit commands before hooks or shell rewriting.
- Curl install scripts are allowed only as reviewed fallback instructions, never silent defaults.
- External retrieval tools must disclose API keys, data movement, indexing cost, and storage location.
- Generated artifacts must not include unknown private tool names from logs.
