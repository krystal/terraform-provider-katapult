# Security Group Framework migration plan

- Status: PR 1 complete; combined Framework migration ready to implement
- Scope: all `katapult_security_group*` resources and data sources
- Delivery: two sequential pull requests

## Outcome

Move the complete Security Group family from Terraform Plugin SDK v2 to the
protocol-v6 Terraform Plugin Framework provider. In the same migration, add
expression-friendly plural rule attributes and retain the existing singular
nested blocks as deprecated compatibility syntax.

Users must be able to upgrade directly from the current blocks-only SDKv2
provider. Unchanged configuration must produce an empty plan, and users must be
free to move between the two rule representations without recreating, updating,
or deleting equivalent remote rules.

The work has two delivery stages:

1. Characterize the SDKv2 behavior with acceptance coverage. This shipped in PR
   #200.
2. Move all six public types to Framework and introduce reversible plural rule
   attributes in one PR.

There will not be a temporary provider version that requires users to migrate
their state through an SDKv2 plural-attribute schema.

## Public types

The migration covers exactly these Terraform types:

| Kind | Type |
| --- | --- |
| Resource | `katapult_security_group` |
| Resource | `katapult_security_group_rule` |
| Data source | `katapult_security_group` |
| Data source | `katapult_security_group_rule` |
| Data source | `katapult_security_group_rules` |
| Data source | `katapult_security_groups` |

Virtual machines, virtual machine groups, tags, address lists, and other objects
referenced by Security Group associations or rule targets are integration
dependencies only.

## Current baseline

Baseline checked against `origin/main` at `5b0b9ee` on 2026-08-20:

- PR #200, `test(security-group): characterize legacy behavior`, is merged.
- All six public types remain registered only in `internal/provider`.
- `main_test.go` still classifies all six types as legacy.
- The legacy family has two resources, four data sources, and 36 Security Group
  acceptance tests.
- There are 36 Security Group VCR cassettes and 31 stable random-ID files under
  `internal/provider/testdata`.
- The generated `next/core` client exposes the required group and rule CRUD and
  list endpoints.
- All six documentation pages already belong to the `Networking` category.

Recheck registrations, generated-client types, dependency versions, test
counts, and cassette counts when implementation begins.

## Settled compatibility contracts

### Direct provider upgrade

- A user may upgrade directly from the current blocks-only SDKv2 provider to
  the final Framework implementation.
- Existing `inbound_rule {}` and `outbound_rule {}` configuration remains valid.
- Genuine protocol-v5 state with unchanged configuration must hand over to
  Framework with an empty plan.
- Security Group and rule API IDs must remain unchanged during handover.
- Users do not need to install any intermediate provider release or rewrite
  configuration during the provider upgrade.

### Remote-object safety

- A representation-only change must issue no Security Group or rule POST,
  PATCH, or DELETE requests.
- A failed inline-rule creation must leave the already-created Security Group
  tracked in Terraform state.
- Out-of-band deletion must remove the missing group or standalone rule from
  Terraform state.
- Resource deletion must treat API not-found responses as success.
- API rule `action` remains unexposed. Deny-rule support is a separate feature.

### Existing values and collections

- `associations` remains set-shaped and order-insensitive.
- Inline rule collections remain list-shaped.
- Rule order and computed IDs remain stable across refreshes.
- Each rule's `targets` remains set-shaped and order-insensitive.
- Protocol input and state comparison remain case-insensitive. Existing state
  and imported API casing remain stable, newly configured casing remains as
  written, and API requests use canonical uppercase protocol values. Framework
  rejects StateFunc-like configured-value normalization because a planned
  uppercase value does not exactly match lowercase configuration.
- Omitted `ports` and `notes` retain their historical empty and null behavior.
- Collection data sources preserve their existing ordering and IDs.

### `external_rules`

Preserve both current transitions:

- `false` to `true`: remove rules previously managed inline, then stop reading
  or reconciling the group's complete rule list.
- `true` to `false`: read existing external rules into state, emit the existing
  two-step guidance, and reconcile those rules only on the following apply.

The `true` to `false` plan must mark the plural rule attributes unknown. The
post-update read then adopts remote rules into attributes without adding absent
configuration blocks or returning state inconsistent with the plan. The next
plan reconciles the adopted rules against configuration.

Immediate deletion or reconciliation during the first apply is destructive and
out of scope.

## PR 1: SDKv2 characterization tests

Status: complete and merged as PR #200.

The merged tests cover:

- Both `external_rules` transitions.
- Standalone-rule coexistence with an externally managed group.
- Group and standalone-rule out-of-band deletion.
- Group collection expansion with `include_rules = true`.
- Omitted and explicitly empty optional rule values.
- Partial inline-rule creation failure after group creation.

These tests and their cassettes are the executable compatibility contract for
the Framework migration. Do not weaken their assertions merely because
Framework renders a diagnostic or state path differently.

## PR 2: Framework migration and plural rule attributes

### Goal

Move all six public types to `internal/v6provider` using the generated
`next/core` client. The group resource must support both rule representations
from its first Framework release:

```terraform
resource "katapult_security_group" "web" {
  name = "web"

  inbound_rules = [
    {
      protocol = "TCP"
      ports    = "22"
      targets  = ["all:ipv4"]
      notes    = "SSH"
    }
  ]

  outbound_rules = []
}
```

The plural attributes are the preferred syntax. The singular blocks remain
fully functional for backwards compatibility, emit deprecation warnings, and
receive no new features.

### Shared implementation

Add one Security Group implementation layer under `internal/v6provider` with:

- Framework models for groups and rules.
- Reusable rule object schema and conversion helpers.
- Generated-client request construction.
- Nullable `ports` and `notes` normalization.
- Association and target set conversion.
- Paginated group and rule readers using the existing v6 pagination helper.
- Stable flattening for direction, IDs, and list order.
- One canonical rule model and semantic fingerprint.
- One reconciliation engine shared by blocks and attributes.

Do not retain production calls through the legacy client and do not implement
separate CRUD paths for the two inline rule representations.

### Group resource schema

Add the preferred attributes:

- `inbound_rules`: optional and computed `ListNestedAttribute`.
- `outbound_rules`: optional and computed `ListNestedAttribute`.

Retain the compatibility blocks:

- `inbound_rule`: deprecated `ListNestedBlock`.
- `outbound_rule`: deprecated `ListNestedBlock`.

Both nested object forms expose the same rule fields:

- `id`: computed.
- `direction`: computed.
- `protocol`: required and case-insensitive. Existing/imported API casing stays
  stable, newly configured casing stays as written, and requests are uppercase.
- `ports`: optional with legacy-compatible empty and null handling.
- `targets`: required and set-shaped.
- `notes`: optional with legacy-compatible empty and null handling.

Use resource-level configuration validation so unknown values defer validation:

- Reject both representations for the same direction.
- Allow inbound and outbound directions to choose representations independently.
- Reject `external_rules = true` with any configured rule collection.
- Reject an allow-all flag with rules configured for the same direction.

Do not use a generic conflict validator that treats a known default of `false`
as configured.

### Selecting a representation

Classify each direction separately from configuration and prior state:

- Configured blocks select the legacy representation.
- A configured plural attribute, including an explicit empty list, selects the
  attribute representation.
- Both representations configured for one direction are invalid.
- If neither is configured, preserve an unambiguous prior representation.
- New resources and imports prefer plural attributes when configuration does
  not select blocks.
- Provider-discovered rules that have no corresponding configured block use the
  plural attributes.

Persist only the selected public representation after apply and refresh. If
null and empty collections leave selection ambiguous, record the smallest
possible marker in resource-private state rather than populating both public
fields.

### Reversible representation migration

Follow the Virtual Machine legacy-disk migration principle: inspect
configuration separately from state, classify the transition before remote
lifecycle work, and retain authoritative object IDs.

Support both directions:

- Deprecated blocks to equivalent plural attributes.
- Plural attributes to equivalent deprecated blocks.

For either direction:

1. Convert prior state and new configuration to canonical rules.
2. Match by existing API ID where available, then by semantic fingerprint of
   direction, protocol, ports, targets, and notes.
3. Copy matched IDs into the planned destination representation.
4. Clear the source representation in the plan.
5. Produce no API operations when the canonical rule collections are equal.
6. Apply the representation change to Terraform state only.
7. Require the following plan to be empty.

Reverting to deprecated blocks emits the block deprecation warning. Do not add a
second reverse-migration warning. Terraform will still show an in-place plan
because the state paths change, but apply must not mutate remote rules.

If representation and rule content change together, preserve IDs for matching
rules and perform only the material create, update, and delete operations.

For semantically identical duplicate rules, preserve the complete ID multiset
and use stable prior-state order for deterministic pairing. No remote mutation
may result from duplicate ambiguity.

### Inline rule reconciliation

Reconciliation operates on canonical rules after representation selection:

- Preserve the ID of an existing matching rule.
- Create each genuinely new rule once and place its returned ID in state.
- Update only material changes.
- Delete each removed rule once.
- Ignore target ordering.
- Preserve IDs when rule lists reorder and semantic matching is unambiguous.
- Match duplicates deterministically.

Set the Security Group ID in Framework state immediately after group creation,
before creating inline rules. This preserves partial state if later rule
creation fails.

### Other Framework parity

- Use the existing null-to-empty set plan modifier for omitted SDKv2 sets such
  as `associations`.
- Use the existing empty-string preservation modifier where nullable API values
  would otherwise disagree with legacy state.
- Normalize protocol and direction for API requests while preserving
  semantically equal configured/prior state casing. Framework does not permit
  SDKv2 `StateFunc`-style rewriting of configured values.
- Add explicit computed `id` attributes where SDKv2 supplied IDs implicitly,
  including the grouped-rule data source.
- Preserve standalone-rule replacement and in-place update behavior.
- Preserve imports for both resources.
- Preserve data-source ordering and `include_rules` behavior.

### Genuine state handover

Generalize the existing migration-test provider factory:

- Retain legacy implementations under test-only aliases.
- Re-register those aliases under their historical public names in a
  protocol-v5 factory.
- Create genuine SDKv2 state in the first acceptance step.
- Switch the same Terraform state directly to the final protocol-v6 provider.
- Require an empty plan before testing further updates or syntax changes.

Cover at minimum:

1. Minimal group.
2. Group with associations and allow-all flags.
3. Group with multiple inbound and outbound blocks, including omitted optional
   values.
4. Group using `external_rules = true` with standalone rule resources.
5. Standalone rule with canonical protocol and empty optional values.
6. Import of both resources followed by stable plans.

Capture group and rule IDs before handover and assert that Framework refresh,
updates, and later representation changes retain them.

### Representation acceptance matrix

| Starting state | Next configuration | Expected result |
| --- | --- | --- |
| SDKv2 blocks | Unchanged Framework blocks | Empty plan; same IDs |
| Framework blocks | Unchanged blocks | Empty plan; deprecation warning |
| Framework blocks | Equivalent plural attributes | State-only change; same IDs; no API mutations |
| Framework blocks | Modified plural attributes | Only material rule changes |
| Plural attributes | Unchanged attributes | Empty plan |
| Plural attributes | Equivalent deprecated blocks | State-only change; same IDs; no API mutations; deprecation warning |
| Plural attributes | Modified deprecated blocks | Only material rule changes; deprecation warning |
| Blocks to attributes to blocks | Equivalent rules | IDs survive the complete round trip |
| Either representation | Reordered targets | Empty plan |
| Either representation | Reordered rules | Stable IDs; no recreation |
| Either representation | Equivalent duplicate rules | Deterministic ID pairing; no API mutations |
| Mixed directions | Old inbound and new outbound, then reversed | Each direction migrates independently |
| Either representation | `external_rules = true` | Existing managed rules removed once |
| `external_rules = true` | No rule configuration | Existing two-step adoption and reconciliation |
| Either representation | Out-of-band deletion | Missing resource removed from state |

For every representation-only step:

- Capture every rule ID before the configuration change.
- Assert the same IDs after apply.
- Unit-test an empty reconciliation operation list.
- Inspect the focused replay cassette and assert there is no Security Group or
  rule POST, PATCH, or DELETE request for that step.
- Require the following plan to be empty.

### Unit tests

Add focused tests for:

- Configuration and state representation classification.
- Both migration directions.
- Null, unknown, omitted, and explicitly empty collections.
- Case-insensitive protocol matching, canonical API protocol projection, and
  ports, notes, and target normalization.
- Unique, reordered, and duplicate semantic matching.
- ID transfer between representations.
- Empty remote operation lists for equivalent migrations.
- Cross-field validation with known and unknown values.
- Pagination and collection ordering.
- Partial creation state preservation.

For material plan behavior, confirm a representative test fails at its intended
assertion under a targeted perturbation, restore the implementation exactly,
and rerun it successfully.

### Existing tests and VCR cassettes

- Move all Security Group unit and acceptance tests to `internal/v6provider`.
- Carry forward every PR #200 characterization scenario.
- Replay moved tests against existing cassettes before considering a recording.
- Adapt only harness and state paths required by Framework and the new plural
  representation.
- Do not weaken behavioral assertions to accommodate implementation differences.

Any new live recording requires explicit authorization. If authorized:

- Record exactly one test at a time.
- Replay that exact test immediately.
- Inspect cassette changes with stats, targeted searches, and bounded ranges.
- Reject unrelated cassette or random-ID drift.

### Registration cutover

Remove each SDKv2 registration and add its Framework registration in the same
commit. The mux must never see duplicate public names.

Update ownership tests so both resources and all four data sources are expected
from the Framework provider. Keep legacy registrations only in the migration
test factory.

### Documentation

- Keep every type in the `Networking` category.
- Make plural attributes the primary Security Group examples.
- Document the deprecated blocks during the compatibility window.
- Add a migration guide covering both syntax directions and the required plan,
  apply, and final empty-plan check.
- State that equivalent representation changes update Terraform state but make
  no remote rule mutations.
- Do not promise a removal release for deprecated blocks before a future major
  version policy is agreed.
- Regenerate documentation from schemas and templates. Do not hand-edit
  generated pages.

### Reviewable commit order

Keep the single PR reviewable with commits aligned to these dependencies:

1. Shared models, conversion helpers, pagination, and semantic matching.
2. Standalone rule resource and four data sources.
3. Group schema, CRUD, and both rule representations.
4. Plan classification, reversible migration, and reconciliation.
5. Genuine v5 handover and representation acceptance tests.
6. Registration cutover, moved legacy tests and cassettes, and generated docs.

Rebase onto fresh `main` before final review. Review the complete combined diff,
not only the last implementation commit.

### Completion gates

- All focused unit tests pass.
- The complete moved Security Group acceptance suite passes in replay.
- Every genuine protocol-v5 handover reaches an empty Framework plan without ID
  changes.
- Both representation migrations retain IDs and perform no remote mutations.
- Both `external_rules` transitions preserve the PR #200 contract.
- Deprecated blocks remain functional and emit their warning.
- Plural attributes accept direct values, variables, comprehensions, and
  conditional lists without dynamic blocks.
- Imports and new resources settle on stable state.
- `mise run docs:generate` changes only intended generated output.
- `mise run docs:verify`, `mise run check`, and `mise run verify` pass.
- `git diff --check` passes.
- Cassette and random-ID changes are explained and limited to intended tests.
- The mux exposes all six public types exactly once.
- Independent review covers schema compatibility, plan consistency, remote
  lifecycle safety, and test quality before the PR is declared ready.

## Deferred work

The following remain outside this migration:

- Exposing Security Group rule `action` or deny rules.
- Renaming or changing the standalone rule resource.
- Replacing inline rules with standalone resources automatically.
- Changing the two-step `external_rules` lifecycle.
- Migrating adjacent resources referenced by associations or targets.
- Removing deprecated singular blocks before a future major release.

## Definition of complete

The migration is complete when all six public types are owned by the Framework
provider, current SDKv2 state upgrades directly with empty plans, both inline
rule syntaxes remain supported, plural attributes are the documented default,
and equivalent representation changes in either direction preserve every remote
rule and rule ID without API mutations.

## Unresolved questions

None. Resource-private state should be added only if implementation tests prove
that configuration and public state cannot distinguish an active representation.
