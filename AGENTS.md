# Repository Guide

This repository builds the Katapult Terraform provider by muxing resources from
the legacy Terraform Plugin SDK and the Terraform Plugin Framework.

## Project Map

- `main.go` combines the two provider implementations into one protocol-v6
  server.
- `internal/provider` contains the remaining legacy Plugin SDK v2 resources and
  data sources, exposed through an upgraded protocol-v5 server.
- `internal/v6provider` contains native protocol-v6 Plugin Framework resources
  and data sources.
- `internal/*/testdata` contains VCR cassettes and their stable random IDs.
- `docs` is generated provider documentation. Change provider schemas or
  templates, then run `mise run docs:generate`; do not hand-edit generated
  output.
- Every resource and data source belongs to a documentation subcategory:
  `Compute`, `Infrastructure`, `Storage`, `Networking`, or `Organization`.
  Maintain the mappings in `templates/resources.md.tmpl` and
  `templates/data-sources.md.tmpl`; bespoke templates must use the matching
  category. `mise run docs:check` rejects uncategorized pages.
- `CONTRIBUTING.md` documents the intentionally gradual v5-to-v6 migration and
  the ownership rules between both implementations.

Check the resource and data-source registration maps before deciding which
provider package owns a change. New types belong in `internal/v6provider`.
When migrating an existing type, remove its legacy registration in the same
change so the mux never sees duplicate type names.

## Code Exploration

Use CodeGraph for structural exploration before reading broad areas of the
codebase manually. When the CodeGraph MCP server is available, prefer its tools
over invoking the CLI directly; use the CLI forms below as fallbacks:

- Use the `codegraph_explore` MCP tool, or `codegraph explore <topic>`, to find
  the relevant symbols, source, and call paths for a feature or behavior.
- Use the `codegraph_node` MCP tool, or `codegraph node <symbol-or-path>`, to
  inspect a symbol or file with its callers, callees, and dependents. Use the
  related MCP tools, or `codegraph callers`, `codegraph callees`, and
  `codegraph impact`, for narrower relationship queries.
- Run `mise run codegraph:init` to initialize a missing index or sync an
  existing one. Continue to use `rg` for exact text, filenames, and config
  searches.

## Setup

- Run `mise run treeboot` in a new worktree to copy supported local config from
  the root checkout, then run the full project setup.
- Run `mise run setup` to download Go dependencies, install the Lefthook Git
  hooks, and initialize or sync CodeGraph. Mise installs missing task tools
  automatically.
- If golangci-lint reports source-code paths from a deleted sibling worktree,
  run `mise exec -- golangci-lint cache clean` before retrying the lint task.
- Keep credentials and developer overrides in ignored `.envrc`,
  `mise.local.toml`, or `.mise.local.toml` files. Never print or commit them.

## Validation

Use the narrowest relevant command while working, then broaden before handoff:

- When deleting tracked Go files, stage the deletions before running
  `mise run format`. The task enumerates index-tracked Go paths, so unstaged
  deletions make `goimports` fail on missing files.

- `mise run build` builds the provider.
- `mise run test` runs the race-enabled unit-test path against recorded
  cassettes.
- `mise run test:acceptance` runs acceptance tests against recorded cassettes.
- Run one acceptance test by narrowing both its package and exact test name:
  `TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_ipv4$' mise run test:acceptance`.
- For slash-separated subtests, anchor every path component. Because the mise
  task delegates to Make, escape end anchors as `$$`, for example:
  `TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultObjectStorage_scenarios$$/^Bucket_minimal$$' mise run test:acceptance`.
- Run a related group with a Go test regular expression, for example:
  `TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_' mise run test:acceptance`.
- Keep `TESTARGS` regexes free of unescaped shell metacharacters such as
  parentheses; the Makefile expands the value unquoted. Prefer exact names or
  simple prefixes, or run grouped cases as separate commands.
- `mise run check` runs the fast local suite, including format, lint, unit,
  dependency, documentation, and offline workflow checks.
- `mise run verify` adds replay acceptance tests, generated-doc freshness,
  GoReleaser validation, and the online action maturity check.
- `mise run workflows:check` validates workflow syntax, security, action pins,
  and the three-day action maturity policy.

`VCR=rec` and `VCR=off` can make live Katapult API requests. Do not use either
without explicit authorization for live acceptance testing. After tests, check
`git status` for cassette or random-ID drift.

`mise run test:acceptance` hardcodes `VCR=replay`; an authorized recording must
use the project environment directly, for example `mise exec -- env VCR=rec
TEST=./internal/v6provider TESTARGS='-run ^TestName$' make testacc`. Prefixing
the Mise task with `VCR=rec` does not override its replay setting.

`retry.StateChangeConf.Refresh` runs in a goroutine. Use the value returned by
`WaitForStateContext`; do not capture refresh results for unsynchronized reads
outside the callback because cancellation can return before a refresh finishes.

The generated API nullable type reports an explicit JSON `null` as specified.
For nullable relationships such as `Disk.VirtualMachineDisk`, check both
`IsSpecified()` and `IsNull()` before calling `Get()`.

The organization VM list endpoint may lowercase hostnames while VM resource
state retains configured casing. Collection tests and consumers must not assume
casing is identical across those views.

The virtual machine package list endpoint rejects `per_page` values above 100.
Keep package pagination at 100 or lower even though the generated parameter type
does not encode that limit.

Released SDKv2 singular disk-template state may contain `template_version = 0`
because its get response omits the version number. Framework handover must accept
that state and refresh it from the disk-template-version endpoint.

Route every `StateChangeConf` delay, minimum timeout, and poll interval through
the provider `Meta` timing helpers, including test sweepers. Compress any
additional wall-clock settling window in replay mode so recorded state
transitions remain fast without changing production timing.

Standalone disk shrink requires a recognizable partition table and a shrinkable
filesystem. Create shrink acceptance fixtures with
`initial_file_system = "ext4"`; blank disks can produce a successful Katapult
task without changing size, and XFS cannot shrink. Always assert API size
convergence after task completion.

Do not infer import adoption from a null API field alone. For create-only values
that the API cannot read back, mark imported state explicitly in resource-private
state, consume that marker on the first adoption plan, and require replacement
for the same null-to-configured transition on provider-created resources.

When stabilizing computed values in resource `ModifyPlan`, classify every
replacement first and leave computed projections unknown on replacement plans.
For in-place plans, copy only known, non-null prior state; copying legacy null
state can cause an inconsistent result when `Read` normalizes it after apply.

Never infer deprecated VM disk ownership from relationship counts during
deletion. Exact disk IDs captured in resource-private state are authoritative;
older VM state without those IDs must migrate additional relationships to
`katapult_disk` and `katapult_disk_assignment` before destroy.

VCR cassette YAML is massive and can exhaust agent context very quickly. Do not
print whole cassette files or review complete cassette diffs by default. Start
with `git diff --stat`, `git diff --numstat`, and `git diff --name-only`, then use
`rg` and bounded file ranges to inspect only the relevant interactions. Treat
unexpected `.cassette.rand_id` changes as generated drift.

Framework Security Group creates can include `allow_all_*` values that SDKv2
applied later with PATCH. Record the affected Framework acceptance case instead
of weakening request matching to accept explicit boolean mismatches.

When the ordered Security Group cassette transport accepts a create through a
compatibility fallback, observe the mutation before returning so synthetic
follow-up reads use the created resource snapshot. Normalize `associations` as
an order-insensitive set, matching the strict request matcher.

## Repository Rules

- Treat mise as the discoverable task interface. The Makefile remains the
  lower-level implementation for commands that have not yet been migrated.
- When migrating SDKv2 resources with `timeouts {}` configuration, use the
  Framework timeouts package's `Block` API to preserve the existing HCL syntax;
  its `Attributes` API requires `timeouts = {}` instead.
- A configured Framework `ListNestedBlock` cannot be planned wholly unknown,
  and its configured fields cannot be rewritten as unknown. During staged
  adoption, preserve configured fields, leave only computed fields unknown,
  and let refresh expose remote differences for the follow-up plan.
- Adding an `Optional` and `Computed` field with a schema default does not
  rewrite existing Framework state before planning, including fields nested in
  lists or blocks. When legacy state must remain plan-empty, increment the
  schema version and use an explicit state upgrader to materialize the default.
- Keep GitHub Actions pinned to full commit SHAs with accurate version comments.
- Preserve the three-day dependency maturity policy in mise, Pinact, and
  Dependabot.
- Give required top-level `id` attributes an explicit description. Otherwise,
  tfplugindocs 0.25.0 renders them as read-only regardless of the schema mode.
- Use Conventional Commit messages.
