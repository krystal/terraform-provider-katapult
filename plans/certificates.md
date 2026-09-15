# Certificate management implementation plan

- Status: ready to implement
- Scope: three certificate resources, two data sources, two actions, the
  go-katapult dependency bump, VCR redaction, tests, examples, generated
  documentation, and agent guidance
- Delivery: one pull request with reviewable commits, landed before the next
  provider release

## Outcome

Terraform users can create, import, inspect, and delete Katapult certificates
of all three issuer types, attach them to load balancer rules through the
existing `certificate_ids` attribute, and trigger re-issue and API token reset
through Terraform actions.

The intended configuration is:

```terraform
resource "katapult_lets_encrypt_certificate" "web" {
  name                 = "www.example.com"
  additional_names     = ["example.com"]
  authorization_method = "dns"

  lifecycle {
    create_before_destroy = true
  }
}

resource "katapult_self_signed_certificate" "internal" {
  name = "app.internal.example.com"
}

resource "katapult_custom_certificate" "uploaded" {
  certificate = file("${path.module}/cert.pem")
  private_key = file("${path.module}/key.pem")
  chain       = file("${path.module}/chain.pem")
}

resource "katapult_load_balancer_rule" "https" {
  load_balancer_id = katapult_load_balancer.web.id
  protocol         = "HTTPS"
  listen_port      = 443
  destination_port = 8080
  certificate_ids  = [katapult_lets_encrypt_certificate.web.id]
}

action "katapult_certificate_reissue" "web" {
  config {
    certificate_id = katapult_lets_encrypt_certificate.web.id
  }
}

data "katapult_certificate" "web" {
  id = katapult_lets_encrypt_certificate.web.id
}

data "katapult_certificates" "all" {}
```

With `http` authorization, one apply can issue and attach the certificate as
long as DNS already points at the load balancer. The port-80 listener must
exist before issuance starts:

```terraform
resource "katapult_load_balancer_rule" "http" {
  load_balancer_id = katapult_load_balancer.web.id
  protocol         = "HTTP"
  listen_port      = 80
  destination_port = 8080
}

resource "katapult_lets_encrypt_certificate" "web" {
  name                 = "www.example.com"
  authorization_method = "http"

  depends_on = [katapult_load_balancer_rule.http]
}
```

## Current baseline

Baseline checked against the `t3code/investigate-certificate-management`
branch at `2462c23` on 2026-09-04:

- The provider pins `github.com/krystal/go-katapult` v0.2.13, whose generated
  `next/core` client is built from the Katapult 2.68.0 schema and exposes only
  `GetCertificate` and `GetOrganizationCertificates`.
- go-katapult v0.2.14 is released. Its client is built from the Katapult
  3.17.0 schema and adds `PostOrganizationCertificates`, `DeleteCertificate`,
  `PostCertificateIssue`, and `PostCertificateResetToken`. The provider builds
  and vets against it with no source changes. The Go module proxy version list
  lags behind the tag, but `go get github.com/krystal/go-katapult@v0.2.14`
  resolves.
- The live Katapult API is version 3.18.0 and has the same certificate
  endpoints. There is no update endpoint, so every configured argument change
  is a replacement.
- `internal/v6provider/certificate.go` already converts load balancer rule
  certificate references to Terraform values. `katapult_load_balancer_rule`
  accepts `certificate_ids` but no test exercises a real certificate.
- No provider resource exposes timestamps today, so there is no existing
  timestamp convention.
- The provider registers no actions. The pinned Plugin Framework v1.19.0,
  terraform-plugin-mux v0.23.1, terraform-plugin-go v0.31.0, and tfplugindocs
  0.25.0 all support actions. Actions require Terraform 1.14 or newer. CI runs
  replay acceptance tests against Terraform 1.9 through 1.15 and latest. The
  local mise pin is Terraform 1.10.
- `internal/vcrtest.RedactSensitiveResponseFields` redacts a fixed set of
  response body keys. No filter touches request bodies. The go-vcr default
  matcher compares only method and URL, so request body redaction cannot break
  replay.
- `mise run docs:check` requires a subcategory on every page under
  `docs/resources` and `docs/data-sources` only.

### Katapult certificate behaviour

Confirmed from the Katapult application source on 2026-09-04. Treat these as
API facts the provider must respect, not as provider rules to re-implement
where the API already rejects bad input:

- `issuer` is one of `lets_encrypt`, `custom`, `self_signed`.
- Let's Encrypt requires `name`, requires `authorization_method` of `dns` or
  `http`, rejects wildcard names with `http`, requires every name to be in a
  verified Katapult DNS zone with `dns`, and rejects `certificate`, `chain`,
  and `private_key`. Issuance runs as a background task. On failure the task
  fails, the certificate enters `issue_failed` with `issue_error`, and Katapult
  retries on its own schedule.
- Self-signed requires `name`, rejects certificate material, and issues an RSA
  4096 certificate valid for one year through a background task. Katapult
  re-issues it a month before expiry.
- Custom requires `certificate` and an RSA `private_key`, accepts an optional
  `chain`, rejects expired certificates, ignores any supplied `name` or
  `additional_names`, derives them from the certificate CN and SANs, and is
  `issued` immediately with a null task.
- Delete is a hard delete with no trash object. It returns 409
  `DeletionRestricted` while a load balancer rule references the certificate.
- Load balancer rules do not validate certificate state, so a `pending`
  certificate can be attached.
- HTTP-01 challenges are answered by Katapult, not by the customer backend.
  Every HTTP and HTTPS listener on every Katapult load balancer, plus the
  port-80 redirect listener when `https_redirect` is enabled, forwards
  `/.well-known/acme-challenge/*` to Katapult's certificates server. The server
  matches the challenge token against the load balancer's organization, so the
  certificate does not need to be attached to any rule. The names must resolve
  to a load balancer in the same organization that listens on port 80, through
  an HTTP rule with `listen_port = 80` or through `https_redirect`. Unknown
  tokens are passed through to the backend.
- With `dns` authorization Katapult creates the `_acme-challenge` TXT records
  in its own DNS service, waits for them to resolve, asks Let's Encrypt to
  validate, and removes them afterwards. A verified parent zone satisfies a
  subdomain name. Domains hosted outside Katapult DNS cannot use `dns`
  authorization.
- Re-issue returns 422 `OperationNotSupported` for custom certificates.
- `certificate_api_url` embeds a bearer token that serves the private key
  without further authentication. Reset-token rotates it.
- `private_key` and `certificate_api_url` are omitted from responses unless
  the API token can view private material.
- Names must match Katapult's hostname pattern, which requires lowercase and a
  dotted suffix. Wildcards are written as `*.example.com`.
- Timestamps are unix seconds. `expires_at` and `last_issued_at` are null
  until first issuance.

## Settled decisions

These were agreed during investigation. Do not reopen them without new
conflicting evidence.

- One resource per issuer type: `katapult_lets_encrypt_certificate`,
  `katapult_self_signed_certificate`, and `katapult_custom_certificate`. Every
  attribute then has a static mode, and the sensitive private key is a
  configured input only on the custom resource.
- Creates wait for issuance by default. The Let's Encrypt resource exposes
  `wait_for_issuance` for configurations where DNS is pointed at the load
  balancer outside Terraform, or where the `dns` zone is verified later.
  Katapult retries issuance on its own schedule, so creating a pending
  certificate and letting it issue later is a supported flow.
- Re-issue and reset-token ship as Terraform actions in this change.
- Private keys use plain `Sensitive` attributes. Write-only attributes are
  deferred because the API returns the key on read regardless.
- Custom certificate acceptance tests record against a committed throwaway
  RSA fixture. Let's Encrypt has no recorded acceptance coverage in this
  change; it gets unit coverage only.
- Documentation subcategory for every new resource, data source, and action is
  `Networking`.

## Contract

### Shared certificate outputs

Every certificate resource and the singular data source expose these
attributes. On resources they are all computed. On the singular data source
`id` is the Required lookup argument and the rest are computed. Values come
from `GetCertificate`:

| Attribute | Type | Notes |
|---|---|---|
| `id` | string | Katapult ID such as `cert_...`. |
| `issuer` | string | `lets_encrypt`, `custom`, or `self_signed`. |
| `state` | string | `pending`, `issuing`, `issued`, or `issue_failed`. |
| `issue_error` | string | Null unless the last issuance failed. |
| `certificate` | string | PEM. Null until issued. Configured input on the custom resource. |
| `chain` | string | PEM. Null when absent. Configured input on the custom resource. |
| `private_key` | string, sensitive | PEM. Null until issued or when the token cannot view private material. Configured input on the custom resource. |
| `certificate_api_url` | string, sensitive | Null when the token cannot view private material. |
| `created_at` | string | RFC 3339 UTC. |
| `expires_at` | string | RFC 3339 UTC. Null until issued. |
| `last_issued_at` | string | RFC 3339 UTC. Null until issued. |

Convert unix seconds to RFC 3339 in UTC through one shared helper. Null API
values stay null in state.

Computed attributes on resources use attribute-level `UseStateForUnknown` so a
plan that only changes `wait_for_issuance` or timeouts stays readable. This is
safe alongside `RequiresReplace` because Terraform re-plans a replacement with
a null prior state, which leaves every computed value unknown; `resource_ip.go`
already pairs the two modifiers. Do not add a resource-level `ModifyPlan` that
copies prior state, which is the case the `AGENTS.md` replacement rule guards
against. Refresh overwrites computed values from the API, which is how
automatic renewals reach downstream references.

### `katapult_lets_encrypt_certificate`

| Attribute | Mode | Notes |
|---|---|---|
| `name` | Required, replace | Primary hostname. |
| `additional_names` | Optional set of strings, replace | Extra hostnames. |
| `authorization_method` | Required, replace | `dns` or `http`, validated at plan time. |
| `wait_for_issuance` | Optional bool, default `true` | When false, create returns after the certificate exists in `pending` state. |
| `timeouts` | `timeouts.Attributes` with `create` and `delete` | Create default 10 minutes. Delete default 2 minutes. |

Add a resource config validator that rejects a wildcard in `name` or
`additional_names` when `authorization_method` is `http`, mirroring the API
rule so the failure surfaces at plan time. Do not validate DNS zone
membership; the API owns that check and the provider has no DNS resources.

Document that `http` authorization needs the names to resolve to a load
balancer in the same organization that listens on port 80, through an HTTP
rule with `listen_port = 80` or a load balancer with `https_redirect`, and
that the certificate does not need to be attached to a rule first. Show the
recommended shape: the certificate resource declares `depends_on` for the
port-80 rule, and the HTTPS rule references the certificate ID, so one apply
issues and attaches the certificate. Recommend `wait_for_issuance = false`
only when DNS is cut over outside Terraform. Document that `dns` authorization
needs the names, or a parent domain, in a verified Katapult DNS zone, that
Katapult manages the challenge records itself, and that domains hosted
elsewhere cannot use it. Document
`create_before_destroy` for rotation because every argument change replaces
the certificate and delete fails while a rule references it.

### `katapult_self_signed_certificate`

| Attribute | Mode | Notes |
|---|---|---|
| `name` | Required, replace | Primary hostname. |
| `additional_names` | Optional set of strings, replace | Extra hostnames. |
| `timeouts` | `timeouts.Attributes` with `create` and `delete` | Create default 5 minutes. Delete default 2 minutes. |

Create always waits for the issuance task.

### `katapult_custom_certificate`

| Attribute | Mode | Notes |
|---|---|---|
| `certificate` | Required, replace | PEM certificate. |
| `private_key` | Required, sensitive, replace | PEM RSA private key. |
| `chain` | Optional, replace | PEM chain. |
| `name` | Computed | Derived by Katapult from the CN. |
| `additional_names` | Computed set of strings | Derived by Katapult from the SANs. |
| `timeouts` | `timeouts.Attributes` with `delete` | Delete default 2 minutes. |

Keep the configured `certificate`, `private_key`, and `chain` values in state
after create, read, and refresh. Do not overwrite them from the API. This
avoids diffs from any server-side normalization and keeps replay working when
cassettes redact the key. Import populates them from the API once, because
there is no configured value yet.

### Lifecycle shared by all three resources

- Create posts to `PostOrganizationCertificates` with the organization
  sub-domain lookup used by other resources, sets `id` in state immediately,
  then waits when the response task is specified, non-null, and waiting is
  enabled. Use `waitForTaskCompletion` with the create timeout. Check the
  nullable task with both `IsSpecified()` and `IsNull()`.
- After a successful wait, read the certificate. If `state` is not `issued`,
  return an error that includes `issue_error`. If the task fails, read the
  certificate and return the same error shape. State already holds the ID, so
  Terraform taints the resource and the next apply replaces it.
- Read maps the API object into the model. A 404 removes the resource from
  state.
- Update exists only to persist `wait_for_issuance` and `timeouts`. It makes
  no API call.
- Delete calls `DeleteCertificate`. Treat 404 as already gone. Surface 409 as
  an error stating that a load balancer rule still references the certificate
  and that `create_before_destroy` avoids this during rotation. Do not retry.
- Import passes the ID through, reads the certificate, and fails with a clear
  message naming the correct resource type when the API `issuer` does not
  match the resource.

### Data sources

`katapult_certificate` takes `id` as Required and exposes every shared output
plus `name`, `additional_names`, and `authorization_method`. Sensitive
attributes stay sensitive.

`katapult_certificates` takes no arguments and returns `certificates`, a list
of objects with `id`, `name`, `issuer`, `state`, `expires_at`, and
`last_issued_at`, following the `katapult_tags` pagination pattern. The list
endpoint returns only those fields. Name lookup is out of scope because
Katapult does not enforce unique certificate names.

### Actions

Both actions use an unlinked action schema and take `certificate_id` as
Required.

- `katapult_certificate_reissue` calls `PostCertificateIssue`, then waits for
  the returned task. Add an optional `timeout` string attribute parsed as a Go
  duration, default `10m`, because action schemas have no timeouts block. Map
  422 to an error stating that custom certificates cannot be re-issued.
- `katapult_certificate_reset_token` calls `PostCertificateResetToken` and
  returns without waiting.

Register them through a `ProviderWithActions` implementation on
`KatapultProvider`. The provider `Configure` method currently assigns only
`ResourceData` and `DataSourceData`; it must also assign `ActionData` wherever
it sets those, or the actions receive a nil `Meta`. Document that
`certificate`, `expires_at`,
`last_issued_at`, and `certificate_api_url` on the owning resource refresh on
the next plan, not within the apply that ran the action.

### Documentation plumbing

- Add the three resources to the `Networking` branch of
  `templates/resources.md.tmpl` and both data sources to
  `templates/data-sources.md.tmpl`.
- Add `templates/actions.md.tmpl` with the same subcategory structure and a
  `Networking` mapping for both actions.
- Extend the `docs:check` task to require a subcategory under `docs/actions`.
- Add `examples/resources/<type>/resource.tf` and `import.sh` for each
  resource, `examples/data-sources/<type>/data-source.tf` for each data
  source, and `examples/actions/<type>/action.tf` for each action.
- Update the documentation category rule in `AGENTS.md` to include actions.

## Implementation steps

Order follows dependency. Each step ends with its own focused verification so
evidence shapes the next step.

### 1. Bump go-katapult and prove the existing suite still replays

Run `go get github.com/krystal/go-katapult@v0.2.14` and `go mod tidy`. The
client schema jumps from Katapult 2.68.0 to 3.17.0, so run `mise run build`,
`mise run test`, and the full `mise run test:acceptance` replay suite before
any certificate code. Investigate any changed response shape rather than
adjusting cassettes. Commit this alone.

### 2. Shared certificate model, conversion, and redaction

Extend `internal/v6provider/certificate.go` with the shared Terraform model,
the API-to-model mapping, the RFC 3339 helper, the issuer-mismatch import
check, and a shared delete helper with the 404 and 409 handling above.

Add `private_key` and `certificate_api_url` to
`vcrtest.sensitiveResponseFields`. Add a save filter that redacts
`private_key` inside JSON request bodies so the custom certificate fixture key
never lands in a cassette. Unit-test the request filter alongside the existing
response filter tests.

Unit-test the mapping with nullable fields both null and set, the timestamp
conversion, and the import mismatch message. Confirm each new test runs by
name.

Unit tests in later steps that need API behaviour the cassettes cannot prove
use the existing `httptest.NewServer` seam that builds a `Meta` against a fake
Katapult API; `resource_object_storage_create_recovery_test.go` and
`resource_virtual_machine_package_test.go` show the pattern. VCR replay cannot
detect a skipped request because unused interactions do not fail a test, so
any test whose purpose is "the provider called this endpoint" must use the
fake server and assert on the received requests.

### 3. `katapult_self_signed_certificate`

Implement the resource, register it, and add a sweeper that deletes
certificates whose name starts with `tf-acc-test`. Use lowercase hostnames
such as `tf-acc-test-<random>.example.com` in tests to satisfy the hostname
pattern.

Record, one cassette at a time:

- `TestAccKatapultSelfSignedCertificate_minimal`: create with `name` only,
  assert `state = issued`, non-empty `certificate`, `issuer = self_signed`,
  RFC 3339 `expires_at`, then `ImportState` with `ImportStateVerify`.
- `TestAccKatapultSelfSignedCertificate_additional_names`: create with
  additional names, then change `name` and assert a `Replace` plan action
  with `plancheck.ExpectResourceAction`.
- `TestAccKatapultLoadBalancerRule_certificate`: self-signed certificate
  attached to an HTTPS rule through `certificate_ids`, asserting the ID round
  trips through the rule resource and rule data source. This is the first
  test of `certificate_ids` against a real certificate.

Add a fake-server unit test for failed issuance: the create succeeds, the task
polls to `failed`, and the certificate reads back as `issue_failed` with an
`issue_error`. Assert that the returned diagnostic contains the error text and
that state still holds the new ID and configured arguments, so Terraform
taints the resource rather than orphaning it and creating another on the next
apply.

At record time, check whether the API returns `private_key` and
`certificate_api_url` for the test token. If it does not, document both
attributes as null unless the token can view private material and keep
assertions tolerant of null.

### 4. `katapult_custom_certificate`

Generate a throwaway fixture once and commit it under
`internal/v6provider/testdata/fixtures/`:

```sh
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 3650 \
  -keyout custom_certificate.key -out custom_certificate.pem \
  -subj '/CN=tf-acc-test-custom.example.com' \
  -addext 'subjectAltName=DNS:tf-acc-test-custom.example.com,DNS:tf-acc-test-custom-alt.example.com'
```

The CN starts with the sweeper prefix so leaked certificates are cleaned up.
Ten years of validity keeps the fixture usable for re-recording. The repository
runs no secret scanner in its hooks or workflows, but GitHub push protection
may flag the key. If it does, mark the fixture as a test key in its file name
and header comment and allow the push through the GitHub prompt.

Implement the resource with configured-value retention as specified. Record
`TestAccKatapultCustomCertificate_minimal`: create from the fixture, assert
`name` and `additional_names` derived from the certificate, `state = issued`,
`issuer = custom`, then import. Always list `private_key` in
`ImportStateVerifyIgnore`: replay serves the redacted response, so the imported
value can never equal the fixture in state. If Katapult normalizes PEM on the
way back, also ignore `certificate` and `chain` and note it in the resource
documentation. Cover the import mapping of all three PEM attributes with a
fake-server unit test instead.

Add an acceptance step that changes `certificate` and asserts a `Replace`
action. Because the fixture is the only committed certificate, generate the
second certificate at test time only if this step proves necessary; otherwise
a unit test asserting `RequiresReplace` on the three PEM attributes is
sufficient.

### 5. `katapult_lets_encrypt_certificate`

Implement the resource with `authorization_method`, `wait_for_issuance`, and
the wildcard-plus-http config validator. Unit-test:

- The request body for `dns` and `http` with and without additional names.
- The plan-time rejection of `authorization_method` values other than `dns`
  and `http`.
- The wildcard-plus-http rejection.
- Create behaviour with `wait_for_issuance = false`, through the fake server:
  the create request is made, no task request follows, and state holds the ID
  with `state = pending`.
- Create behaviour with waiting enabled, through the fake server: the task is
  polled to `completed` and the final read populates the computed outputs.

No cassette is recorded for this resource in this change. Note in the resource
documentation that issuance depends on prerequisites the certificate resource
cannot create: for `dns`, a verified Katapult DNS zone; for `http`, DNS
pointing at a Katapult load balancer with a port-80 listener, which can be
managed in the same configuration.

### 6. Data sources

Implement `katapult_certificate` and `katapult_certificates` and register
them. Record `TestAccKatapultDataSourceCertificate_minimal` and
`TestAccKatapultDataSourceCertificates_minimal` against a self-signed
certificate created in the same configuration. Assert the plural data source
contains the created ID with matching `issuer` and `state`.

Add a fake-server unit test for the plural data source with two pages:
assert the page parameters sent, that both pages are aggregated, that null
`expires_at` and `last_issued_at` map to null, and that an error on the second
page fails the read.

Once every certificate schema exists, add a unit test that walks the three
resource schemas and the singular data-source schema and asserts `Sensitive`
on `private_key` and `certificate_api_url`, including inside nested objects.
Redaction and behavioural tests cannot catch a missing flag.

### 7. Actions and Terraform version pin

Implement both actions and the `Actions()` provider method. Bump the mise
`terraform` pin to `1.15` so local acceptance runs execute action tests.

Record, gated with `tfversion.SkipBelow(tfversion.Version1_14_0)`:

- `TestAccKatapultCertificateReissue_self_signed`: self-signed certificate
  plus a `terraform_data` resource whose `lifecycle.action_trigger` runs the
  reissue action `after_create`. Assert the apply succeeds and the
  certificate remains `issued` on refresh.
- `TestAccKatapultCertificateResetToken_self_signed`: same shape for reset
  token. Do not assert on `certificate_api_url` content because cassettes
  redact it.

The recorded tests prove the actions run end to end under Terraform, but
replay cannot prove the API was called. Add fake-server unit tests for both
`Invoke` methods that assert the endpoint path and request body, that reissue
polls the task to completion and honours `timeout`, that a failed task returns
an error, and that 422 on reissue maps to the custom-certificate message.

Confirm the muxed provider advertises action schemas by running one action
test against the built provider, not only the Framework server in isolation.

### 8. Documentation, examples, and agent guidance

Add the templates, examples, `docs:check` extension, and generated docs. Run
`mise run docs:generate` and `mise run docs:verify`.

Update `AGENTS.md` with the smallest durable rules learned here:

- Actions require Terraform 1.14 and their acceptance tests must skip below
  it.
- `docs:check` covers `docs/actions`.
- Custom certificate tests use the committed fixture and the request body
  redaction filter.
- Custom certificate PEM inputs are retained from configuration, not the API.

## Verification strategy

Use the narrowest evidence while implementing, then broaden before handoff.

### Focused checks

Run the certificate unit tests after each step. Run each new acceptance test
in replay immediately after recording it:

```sh
TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSelfSignedCertificate_' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultCustomCertificate_minimal$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultLoadBalancerRule_certificate$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultDataSourceCertificate' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultCertificateRe' \
  mise run test:acceptance
```

Confirm from the output that each named test ran. Check `git status` after
every recording for cassette and random-ID drift, and inspect new cassettes
with `rg` for `BEGIN.*PRIVATE KEY` and `certificate_api_url` values to prove
redaction worked before committing.

Recording rules: record one cassette at a time, replay it, inspect it, then
continue. Recording uses the project environment directly:

```sh
mise exec -- env VCR=rec TEST=./internal/v6provider \
  TESTARGS='-run ^TestName$' make testacc
```

Before the first recording, confirm the test organization has headroom under
its certificate limit.

### Broad checks

- `mise run check` after step 1 and again before handoff.
- Full `mise run test:acceptance` replay after step 1 and before handoff.
- `mise run verify` before handoff.
- `git diff --check`.

## Review focus

Independent review should concentrate on:

- Nullable task handling on create, including custom certificates with a null
  task and Let's Encrypt with waiting disabled.
- Tainted-state behaviour when issuance fails, and the error including
  `issue_error`.
- Configured PEM retention on the custom resource across create, read,
  refresh, and import.
- Import issuer mismatch producing a clear, actionable error.
- Sensitive marking on `private_key` and `certificate_api_url` everywhere they
  appear, including nested data-source objects.
- Redaction of private keys in both response and request bodies of every new
  cassette.
- Action version gating and the mux server advertising action schemas.
- Generated documentation matching the contract above, with every new page
  categorized.

## Completion gates

- go-katapult v0.2.14 is pinned and the pre-existing replay suite passes
  unchanged.
- Self-signed and custom certificates create, import, replace, and destroy
  through recorded acceptance tests.
- A real certificate round-trips through `katapult_load_balancer_rule`
  `certificate_ids`.
- Let's Encrypt request construction, plan-time validation, and both
  `wait_for_issuance` branches are unit-tested against the fake server, and
  the missing live coverage is recorded in the resource documentation and pull
  request.
- Failed issuance after a successful create leaves the ID in state and returns
  a diagnostic containing `issue_error`, proven by a fake-server test.
- Both data sources have recorded acceptance coverage, and the plural data
  source aggregates multiple pages in a fake-server test.
- Both actions invoke successfully under Terraform 1.14 and later, skip
  cleanly on older versions, and have fake-server tests proving the request
  and task wait.
- A schema test asserts `Sensitive` on every `private_key` and
  `certificate_api_url` attribute.
- No cassette contains a private key or a certificate API URL token.
- Every new resource, data source, and action page has a `Networking`
  subcategory and `mise run docs:verify` passes.
- `mise run check`, `mise run verify`, and `git diff --check` pass.
- The worktree contains no unexplained cassette or random-ID drift.

## Deferred work

- Let's Encrypt recorded acceptance coverage, pending a verified DNS zone in
  the test organization.
- Katapult DNS zone and record resources, which `dns` authorization depends
  on.
- Write-only private key input for custom certificates.
- Delete retry while a rule still references the certificate, if rotation
  without `create_before_destroy` proves common.
- Client-side filtering on `katapult_certificates`.

## Unresolved questions

- Whether the test organization's API token can view private material. Step 3
  resolves this at record time and adjusts documentation and assertions.
- Whether Katapult returns custom certificate PEM exactly as uploaded. Step 4
  resolves this at record time.
