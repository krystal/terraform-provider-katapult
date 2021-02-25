# Katapult Terraform Provider

- [Provider Documentation](https://registry.terraform.io/providers/krystal/katapult/latest/docs)
- Katapult website: [https://katapult.io](https://katapult.io)
- Terraform website: [https://www.terraform.io](https://www.terraform.io)

<img src="https://cdn.rawgit.com/hashicorp/terraform-website/master/content/source/assets/images/logo-hashicorp.svg" width="600px">

## Status

This provider is still in the early stages of development. As we add and expand
functionality to support more of Katapult's features, we will do our best to
avoid breaking changes. If breaking changes are required, they will be clearly
listed in the release notes and changelog.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) 0.14.x
- [Go](https://golang.org/dl/) 1.16 (to build the provider plugin)

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

To work on the provider, you will first need [Go](https://golang.org/dl/) (1.16
or later is _required_), and also
[Terraform](https://www.terraform.io/downloads.html) 0.14.x or later for
acceptance tests.

### Make Targets

- `make build` — Build provider binary into `bin/terraform-provider-katapult`
- `make install` — Build provider binary, and install it to
  `~/.terraform.d/plugins/registry.terraform.io/krystal/katapult/{VERSION}/`,
  allowing Terraform to use the custom builds.
- `make test` — Run unit tests.
- `make testacc` — Run acceptance tests. By default it prevent requests to
  Katapult's API to create real resources, and instead playback previously
  record requests. To enable real requests against Katapult, set the `VCR`
  environment variable to `rec` to record requests, or `off` to disable the VCR
  request recording/playback all together.
