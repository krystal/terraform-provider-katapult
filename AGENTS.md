# Repository Guide

This repository builds the Katapult Terraform provider by muxing resources from
the legacy Terraform Plugin SDK and the Terraform Plugin Framework.

## Project Map

- `main.go` combines the two provider implementations into one protocol-v6
  server.
- `internal/provider` contains legacy Plugin SDK resources and data sources.
- `internal/v6provider` contains Plugin Framework resources and data sources.
- `internal/*/testdata` contains VCR cassettes and their stable random IDs.
- `docs` is generated provider documentation. Change provider schemas or
  templates, then run `make docs`; do not hand-edit generated output.

Check the resource and data-source registration maps before deciding which
provider package owns a change. Update both implementations only when behavior
is intentionally shared.

## Setup

- Run `mise run treeboot` in a new worktree to copy supported local config from
  the root checkout.
- Run `mise run setup` to install locked tools and the Lefthook Git hooks.
- Keep credentials and developer overrides in ignored `.envrc`,
  `mise.local.toml`, or `.mise.local.toml` files. Never print or commit them.

## Validation

Use the narrowest relevant command while working, then broaden before handoff:

- `make build` builds the provider.
- `VCR=replay make test` runs the race-enabled unit-test path against recorded
  cassettes.
- `VCR=replay make testacc` runs acceptance tests against recorded cassettes.
- Run one acceptance test by narrowing both its package and exact test name:
  `VCR=replay make testacc TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_ipv4$'`.
- Run a related group with a Go test regular expression, for example:
  `VCR=replay make testacc TEST=./internal/v6provider TESTARGS='-run ^TestAccKatapultIP_'`.
- `make lint` and `make lint-provider` run Go and provider-specific linting.
- `make check-tidy` verifies `go.mod` and `go.sum` are tidy.
- `make check-docs` validates generated documentation; `make docs` regenerates
  it.
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

- Keep build, test, Go lint, and docs tasks in the `Makefile`; use mise for
  cross-language tooling, setup, worktree bootstrap, and workflow checks.
- Keep GitHub Actions pinned to full commit SHAs with accurate version comments.
- Preserve the three-day dependency maturity policy in mise, Pinact, and
  Dependabot.
- Use Conventional Commit messages.
