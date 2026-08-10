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
  templates, then run `make docs`; do not hand-edit generated output.
- `CONTRIBUTING.md` documents the intentionally gradual v5-to-v6 migration and
  the ownership rules between both implementations.

Check the resource and data-source registration maps before deciding which
provider package owns a change. New types belong in `internal/v6provider`.
When migrating an existing type, remove its legacy registration in the same
change so the mux never sees duplicate type names.

## Setup

- Run `mise run treeboot` in a new worktree to copy supported local config from
  the root checkout.
- Run `mise run setup` to install locked tools and the Lefthook Git hooks.
- Keep credentials and developer overrides in ignored `.envrc`,
  `mise.local.toml`, or `.mise.local.toml` files. Never print or commit them.

## Validation

Use the narrowest relevant command while working, then broaden before handoff:

- `mise run build` builds the provider.
- `mise run test` runs the race-enabled unit-test path against recorded
  cassettes.
- `mise run test:acceptance` runs acceptance tests against recorded cassettes.
- Run one acceptance test by narrowing both its package and exact test name:
  `TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_ipv4$' mise run test:acceptance`.
- Run a related group with a Go test regular expression, for example:
  `TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_' mise run test:acceptance`.
- `mise run check` runs the fast local suite, including format, lint, unit,
  dependency, documentation, and offline workflow checks.
- `mise run verify` adds replay acceptance tests, generated-doc freshness,
  GoReleaser validation, and the online action maturity check.
- `mise run workflows:check` validates workflow syntax, security, action pins,
  and the three-day action maturity policy.

`VCR=rec` and `VCR=off` can make live Katapult API requests. Do not use either
without explicit authorization for live acceptance testing. After tests, check
`git status` for cassette or random-ID drift.

VCR cassette YAML is massive and can exhaust agent context very quickly. Do not
print whole cassette files or review complete cassette diffs by default. Start
with `git diff --stat`, `git diff --numstat`, and `git diff --name-only`, then use
`rg` and bounded file ranges to inspect only the relevant interactions. Treat
unexpected `.cassette.rand_id` changes as generated drift.

## Repository Rules

- Treat mise as the discoverable task interface. The Makefile remains the
  lower-level implementation for commands that have not yet been migrated.
- Keep GitHub Actions pinned to full commit SHAs with accurate version comments.
- Preserve the three-day dependency maturity policy in mise, Pinact, and
  Dependabot.
- Use Conventional Commit messages.
