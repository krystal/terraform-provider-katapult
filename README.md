<p align="center">
  <a href="https://registry.terraform.io/providers/krystal/katapult/latest/docs"><img alt="logo" width="325px" src="https://github.com/krystal/terraform-provider-katapult/raw/main/img/logo.svg" /></a>
</p>

<h1 align="center">
  Katapult Terraform Provider
</h1>

<p align="center">
  <a href="https://github.com/krystal/terraform-provider-katapult/releases">
    <img src="https://img.shields.io/github/v/tag/krystal/terraform-provider-katapult?label=release" alt="GitHub tag (latest SemVer)">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/actions/workflows/ci.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/krystal/terraform-provider-katapult/ci.yml?logo=github" alt="Build Status">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/actions/workflows/nightly.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/krystal/terraform-provider-katapult/nightly.yml?logo=github&label=nightly%20acceptance%20tests" alt="Nightly Acceptance Tests">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/issues">
    <img src="https://img.shields.io/github/issues-raw/krystal/terraform-provider-katapult.svg?style=flat&logo=github&logoColor=white" alt="GitHub issues">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/pulls">
    <img src="https://img.shields.io/github/issues-pr-raw/krystal/terraform-provider-katapult.svg?style=flat&logo=github&logoColor=white" alt="GitHub pull requests">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/blob/master/LICENSE">
    <img src="https://img.shields.io/github/license/krystal/terraform-provider-katapult.svg?style=flat" alt="License Status">
  </a>
</p>

## Documentation

- Katapult Terraform Provider:
  [https://registry.terraform.io/providers/krystal/katapult/latest/docs](https://registry.terraform.io/providers/krystal/katapult/latest/docs)
- Katapult website: [https://katapult.io](https://katapult.io)
- Terraform website: [https://www.terraform.io](https://www.terraform.io)

## Status

This provider is still in the early stages of development. As we add and expand
functionality to support more of Katapult's features, we will do our best to
avoid breaking changes. If breaking changes are required, they will be clearly
listed in the release notes and changelog.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) 1.9 or later. Earlier
  versions may work, but are untested.

## Using the Provider

To quickly get started with using the provider, please refer to the
[official documentation](https://registry.terraform.io/providers/krystal/katapult/latest/docs)
hosted on Terraform Registry.

If you are new to Terraform itself, please refer to the official
[Terraform Documentation](https://www.terraform.io/docs/index.html).

## Build the Provider

Clone the provider to your machine, for example:
`~/Projects/terraform-provider-katapult`

```bash
git clone git@github.com:krystal/terraform-provider-katapult.git ~/Projects/terraform-provider-katapult
```

Enter the provider directory and build the provider:

```bash
cd ~/Projects/terraform-provider-katapult
mise run build
```

## Developing the Provider

### Requirements

- [Go](https://go.dev/dl/) 1.26 or later.
- [Terraform](https://www.terraform.io/downloads.html) 1.9 or later.

### Rules

- Always follow the
  [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/) standard
  when writing your commit messages. This will among other things, ensure
  relevant changes are automatically added to the Changelog.

The provider is in a gradual migration from the Terraform Plugin SDK v2 and
protocol v5 to the Plugin Framework and protocol v6. See
[CONTRIBUTING.md](CONTRIBUTING.md#provider-v5-to-v6-migration) before adding or
migrating a resource or data source.

### Development Tasks

Run `mise tasks` to discover the supported task surface. The most useful entry
points are:

- `mise run build` — build the provider binary.
- `mise run test` — run race-enabled unit tests using VCR replay.
- `mise run test:acceptance` — run acceptance tests using VCR replay.
- `mise run check` — run the fast local formatting, linting, unit-test,
  dependency, documentation, and workflow checks.
- `mise run verify` — run the broad pre-handoff suite, including replay
  acceptance tests and generated-output checks.

The Makefile remains available as the lower-level implementation and for
specialized targets such as installation, sweeping, coverage, and the
development container.

### Make Targets

- `make build` — Build provider binary into `bin/terraform-provider-katapult`
- `make install` — Build provider binary, and install it to
  `~/.terraform.d/plugins/registry.terraform.io/krystal/katapult/{VERSION}/`,
  allowing Terraform to use the custom builds.
- `make test` — Run unit tests.
- `make testacc` — Run acceptance tests. By default it prevents requests to
  Katapult's API to create real resources, and instead plays back previously
  record requests. To enable real requests against Katapult, set the `VCR`
  environment variable to `rec` to record requests, or `off` to disable the VCR
  request recording/playback all together.
- `make lint` — Run golangci-lint to lint all Go code.
- `make docs` — Re-generate docs into `./docs` folder.

## Releasing the Provider

Creating a new release is a semi-manual process with some tools to help along
the way.

All Terraform providers must follow [Semantic Versioning](https://semver.org),
and this provider is no different. To help make this easier, we use the
[Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/) commit
message format, along with Google's
[release-please](https://github.com/googleapis/release-please) tool.

The end result is, that whenever `main` changes, release-please will create or
update a release pull request as needed. The PR contains updates to the
changelog, and has automatically calculated and bumped the version as needed
based on Conventional Commits and Semantic Versioning.

Merging the release PR, will trigger a full release with binaries being built
and published to a GitHub Release. However, because the release is created and
published by release-please before goreleaser runs and builds binary assets, the
Terraform Registry may complain it found no binary assets. In that case forcing
a re-sync under the provider settings in Terraform Registry should resolve it.
