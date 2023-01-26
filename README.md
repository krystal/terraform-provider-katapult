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
    <img src="https://img.shields.io/github/workflow/status/krystal/terraform-provider-katapult/CI.svg?logo=github" alt="Build Status">
  </a>
  <a href="https://github.com/krystal/terraform-provider-katapult/actions/workflows/nightly.yml">
    <img src="https://img.shields.io/github/workflow/status/krystal/terraform-provider-katapult/Nightly%20Acceptance%20Tests.svg?logo=github&label=nightly%20acceptance%20tests" alt="Nightly Acceptance Tests">
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

- [Terraform](https://www.terraform.io/downloads.html) 1.0 or later.

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
make build
```

## Developing the Provider

### Requirements

- [Go](https://golang.org/dl/) 1.19 or later.
- [Terraform](https://www.terraform.io/downloads.html) 1.0 or later.

### Rules

- Always follow the
  [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/) standard
  when writing your commit messages. This will among other things, ensure
  relevant changes are automatically added to the Changelog.

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

## Releasing the Provider

Creating a new release is a semi-manual process with some tools to help along
the way.

All Terraform providers must follow [Semantic Versioning](https://semver.org),
and this provider is no different. To help make this easier, we use the
[Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/) commit
message format in combination with a tool called
[`standard-version`](https://github.com/conventional-changelog/standard-version)
for automatic version bumping and changelog generation.

We use [GitHub Actions](https://github.com/features/actions) and
[goreleaser](https://goreleaser.com) to build, sign, and publish binaries as
GitHub Releases for all supported platforms.

### Steps

In your local development working directory:

1. Ensure your working directory is on the `main` branch and fully to to date.
2. Run `make next-version` to preview both the changelog update and next version
   based on commits since the last release. If you need to override/customize
   the changelog and/or automatically determined version, please see
   [Customize Changelog or Version](#customize-changelog-or-version) below.
3. Run `make new-version` from the root of your working directory. This will use
   `standard-version` to:
   1. Determine the current version by looking at the most recent Semantic
      Version formatted git tag.
   2. Look at the list of commits since the last version, and based on
      conventional commit standards, determine if this next version is a new
      PATCH, MINOR, or MAJOR version.
   3. Update `CHANGELOG.md` based on all new commit messages of types `feat`,
      `fix`, and `docs` since the last release.
   4. Commit the changes to `CHANGELOG.md` with a commit message of:
      ```
      chore(release): <VERSION>
      ```
   5. Tag the release commit as `v<VERSION>`.
4. Push the release commit and tag with:
   ```
   git push --follow-tags origin main
   ```
5. Wait for GitHub Actions to complete running for the tag you just pushed. The
   final step called "release" will create a draft GitHub Release for the new
   version.
6. Go to
   [Releases](https://github.com/krystal/terraform-provider-katapult/releases)
   and edit the new draft release. Typically the release description/body should
   be the same as the new changelog content for that version. So feel free to
   copy/paste it. This will be automated at some point in the future.
7. Publish the draft release.
8. Wait 5-10 minutes, and the new version should appear on the
   [Terraform Registry](https://registry.terraform.io/providers/krystal/katapult/latest).

### Customize Changelog or Version

To customize the changelog and/or version number picked by `standard-version`,
instead of running `make new-version`, just run standard-version manually with
additional options.

Examples:

- Preview what standard-version will do with `--dry-run`:
  ```
  npx standard-version --dry-run
  ```
- Customize changelog, by skipping the commit and tag steps:
  ```
  npx standard-version --skip.commit --skip.tag
  ```
- Override next version with the `-r` flag to be `v2.3.1`:
  ```
  npx standard-version -r 2.3.1
  ```

If you skipped the commit and/or tag stages, you will need to perform them
manually.

The Git tag **MUST** start with a `v` prefix, and be fully semantic version
compatible.

The commit message, assuming `v2.3.1`, should be:

```
chore(release): 2.3.1
```

Once done, simply push the commit and tag, and wait for the draft GitHub release
to be created.
