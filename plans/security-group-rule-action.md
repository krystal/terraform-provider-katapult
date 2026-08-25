# Security Group rule action implementation plan

- Status: ready to implement
- Scope: Security Group rule resources, inline rules, data sources, state
  compatibility, tests, examples, and generated documentation
- Related issue: [#168](https://github.com/krystal/terraform-provider-katapult/issues/168)
- Delivery: one follow-up pull request before the next provider release

## Outcome

Add full Terraform support for Katapult Security Group rule actions. Users can
set `action` to `allow` or `deny` on standalone and inline rules. Omitting the
attribute produces `allow` in the Terraform plan, state, and API request.

Existing users must be able to upgrade without editing configuration. Security
Group rules created by a released provider version have no `action` field in
Terraform state, but those rules use Katapult's existing allow behavior. The
Framework provider must refresh them as `action = "allow"`, retain every remote
rule ID, perform no API mutation, and produce an empty plan.

The intended configuration is:

```terraform
resource "katapult_security_group" "ssh" {
  name               = "ssh"
  allow_all_outbound = true

  inbound_rules = [
    {
      action   = "deny"
      protocol = "TCP"
      ports    = "22"
      targets  = ["100.64.0.0/10"]
    },
    {
      action   = "allow"
      protocol = "TCP"
      ports    = "22"
      targets  = ["all:ipv4"]
    },
  ]
}
```

Rule list order has no firewall meaning. Katapult evaluates all matching user
deny rules before user allow rules, then applies an implicit deny-all rule if no
user rule matched. The provider must not sort rules by action or expose the
implicit deny-all rule in configuration or state.

## Current baseline

Baseline checked against `origin/main` at `d934d82` on 2026-08-25:

- PR #202 moved the complete Security Group family to the protocol-v6 Terraform
  Plugin Framework provider.
- The latest published provider is v0.0.20, which predates the Framework
  migration. There is no published intermediate Framework Security Group state
  shape to support.
- `github.com/krystal/go-katapult` v0.2.13 already exposes `action` on rule
  arguments and create, update, detail, and list responses. Its action enum
  contains `allow` and `deny`.
- Both managed rule paths currently omit `action` from API requests and ignore
  it in API responses.
- All current recorded Security Group rule mutation requests omit `action`.
  Twenty-seven of the 39 Security Group cassettes contain no `action` value at
  all; the remaining recent recordings include `"action":"allow"` in API
  responses. Production normalization, rather than wholesale cassette
  rewriting, must keep both response shapes usable.
- The existing genuine protocol-v5 handover test creates SDKv2 state and then
  switches the same Terraform state to the Framework provider.
- Live recording of new cassettes and re-recording of existing cassettes is
  authorized when implementation needs it. Record or re-record exactly one
  cassette at a time, then replay and inspect it before starting another.
- The prior Framework migration plan deferred action support and said deprecated
  singular blocks would receive no new features. This plan supersedes those two
  statements for `action`. All rule representations need the field so state and
  remote semantics cannot disagree.

Recheck the dependency version, latest release, and relevant test names before
implementation begins.

## Settled contract

### Schema

Expose `action` everywhere a Security Group rule appears:

- `katapult_security_group_rule` resource.
- `inbound_rules` and `outbound_rules` nested attributes.
- Deprecated `inbound_rule` and `outbound_rule` nested blocks.
- `katapult_security_group_rule` data source.
- Rule objects returned by `katapult_security_group_rules`.
- Rule objects returned by `katapult_security_group` when rules are included.
- Rule objects returned by `katapult_security_groups` when rules are included.

Use a shared resource attribute with this behavior:

- Optional and computed.
- Static default of `string(core.Allow)`.
- Accept only lowercase `allow` and `deny` with a case-sensitive validator.
- Send the resolved action explicitly on create and update.
- Store the normalized API action after create, read, and import. After a
  successful update, store the validated planned action, matching the existing
  `ports` and `notes` convention; the next read normalizes the API response.
- Treat an omitted API action and a null or empty action decoded from released
  SDKv2 state as `allow`.

Data-source action fields are computed and use the same response normalization.

### Firewall semantics

For each connection, Katapult applies this decision order:

1. A matching user deny rule drops the connection.
2. If no deny matched, a matching user allow rule permits the connection.
3. If no user rule matched, the implicit deny-all rule drops the connection.

Configuration order and API response order do not change this behavior. The
provider continues to preserve stable list state and IDs where practical, but
it must not describe list order as firewall precedence.

`action` is part of a rule's semantic identity. Two rules with the same
direction, protocol, ports, targets, and notes but different actions are
distinct. Changing only `action` updates the existing remote rule in place and
retains its ID.

### Existing behavior that remains unchanged

- `external_rules` continues to define whether the group resource owns the
  complete inline rule set.
- Standalone rules continue to require `external_rules = true` on a group that
  Terraform also manages.
- Existing allow-all and inline-rule conflicts remain unchanged. Supporting
  deny exceptions alongside `allow_all_inbound` or `allow_all_outbound` is not
  part of this work.
- Targets remain set-shaped and order-insensitive.
- Inline rule collections remain list-shaped, with reorder handling based on
  semantic matching and stable IDs.
- Protocol and direction casing behavior remains unchanged.
- Omitted `ports` and `notes` retain their current empty and null behavior.
- Existing `external_rules` adoption and deletion lifecycles remain unchanged.

## Existing-state compatibility contract

The implementation must satisfy this matrix before it is considered safe to
release:

| Starting state | Configuration | Required result |
| --- | --- | --- |
| Released SDKv2 standalone allow rule | `action` omitted | Refresh stores `allow`; same ID; no mutation; empty plan |
| Released SDKv2 inline allow rules | `action` omitted in singular blocks | Every rule stores `allow`; same IDs; no mutation; empty plan |
| Released SDKv2 rule state | First Framework plan or apply uses `-refresh=false` | Missing or null action canonicalizes to `allow`; same IDs; no mutation |
| Released SDKv2 group without rules | Unchanged | Empty plan; no synthetic rule objects |
| Released SDKv2 group with `external_rules = true` | Unchanged | Group does not adopt or mutate external rules |
| New Framework standalone rule | `action` omitted | Plan, request, and state contain `allow` |
| New Framework inline rule | `action` omitted | Plan, request, and state contain `allow` |
| Framework-created allow rule | Explicit `action = "allow"` added | Empty plan; same ID; no API mutation |
| Existing allow rule | Changed to `deny` | One PATCH; same ID; state becomes `deny` |
| Existing deny rule | Changed to `allow` | One PATCH; same ID; state becomes `allow` |
| Imported allow rule | Matching configuration omits action | Import refresh stores `allow`; following plan is empty |
| Imported deny rule | Matching configuration sets `action = "deny"` | Import refresh stores `deny`; following plan is empty |
| Allow and deny rules with otherwise equal fields | Either list order | Distinct IDs retained; no cross-pairing or recreation |
| Same rules in a different list order | Actions unchanged | No remote mutation; IDs follow their semantic rules |

The compatibility guarantee targets released provider state. Because no
Framework Security Group implementation has been released, do not add a
Framework state version or `UpgradeState` implementation by default. The
genuine SDKv2 handover test is the deciding evidence. If the nested object type
cannot decode or refresh old state cleanly, stop and design the smallest
explicit state upgrade before continuing.

## Implementation steps

### 1. Establish a compile-safe test seam and failing behavioral tests

Tests cannot refer to an `Action` field before it exists. Add the Go field
declarations to the shared canonical model, Terraform models, standalone
resource model, and data-source rule model first, without projecting or sending
the value. This is a temporary working step, not a commit or validation
boundary.

Then add the smallest focused tests:

- Extend the genuine v5-to-v6 migration acceptance test with `action =
  "allow"` assertions on the standalone rule, both inline representations, and
  all rule-returning data sources.
- Add a post-handover `PlanOnly` step that changes omitted action to explicit
  `action = "allow"` and still requires an empty plan.
- Add an import-state check for inline group rules. Existing group import tests
  ignore all four rule representation attributes and cannot prove imported
  actions without a dedicated check.
- Add unit cases proving action participates in fingerprints, ID transfer, and
  reconciliation classification.
- Add a case with otherwise identical allow and deny rules so an implementation
  that ignores action cross-pairs their IDs or misses a required update.
- Extend `securityGroupRulePropertiesEqual` coverage so an action-only change is
  unequal and reaches PATCH.
- Add canonicalization coverage proving a null or empty action decoded from
  released SDKv2 state becomes `allow` and fingerprints equal explicit allow.
- Add schema tests for the default and accepted values.
- Add request projection and response normalization tests for allow, deny, and
  an omitted API action.
- Add matcher tests before changing replay compatibility. Explicit allow must
  match a legacy request that omitted action. Explicit deny must not match it.

Confirm behavioral tests fail at their intended assertions. Add the shared
object type and schema attributes together in step 2 so state conversion remains
valid. For schema or migration behavior that cannot reach an assertion before
that atomic edit, use one representative targeted perturbation, restore the code
exactly, and rerun the focused test.

### 2. Extend shared types, schemas, normalization, and read paths atomically

Update the shared rule code, both resource schemas, and data-source schema in one
compile-safe edit:

- Add `action` to `securityGroupRuleObjectType`,
  `securityGroupRuleAttributes`, and `computedSecurityGroupRuleAttributes` at
  the same time. These definitions describe the same nested value and must not
  diverge.
- Make resource action optional and computed, default it with
  `stringdefault.StaticString(string(core.Allow))`, and accept only lowercase
  `allow` and `deny`.
- Read action from nested values and write it into planned and refreshed rule
  objects.
- Include action in the unknown-configured-field check so unknown expressions
  defer semantic matching until apply.
- Normalize known null or empty Terraform state action to `allow`. Do not turn
  an unknown configured value into a default during semantic matching.
- Normalize a nil API action to `allow` and otherwise use the lowercase API
  value.
- Include canonical lowercase action in the semantic fingerprint.
- Populate action in the standalone resource read path and every direct,
  grouped, and collection data-source read path.
- Preserve API list ordering without attaching firewall meaning to it.

Keep both API and state normalization in the shared layer. Standalone, inline,
and data-source paths must not invent separate defaulting rules. Run the unit,
schema, data-source, and genuine handover replay tests before adding action to
mutation requests.

### 3. Preserve legacy cassette replay before projecting action into requests

Update `internal/v6provider/security_group_characterization_matcher_test.go`
before any POST or PATCH begins sending action:

- Normalize explicit `"action":"allow"` as equivalent to a missing action only
  when comparing request properties.
- Implement this as a value-aware rule. Do not add `action` to
  `securityGroupLegacyOmittableJSONProperty`, because that helper discards a
  listed property regardless of value and would also accept deny as omission.
- Never normalize deny to omission.
- Keep unknown actual properties rejected.
- Ensure compatibility-matched mutations still call `observeMutation` before a
  synthetic read.
- Ensure old responses without action become allow through production response
  normalization rather than cassette rewriting.

Do not rewrite existing cassettes merely to add default allow requests. The
matcher compatibility is part of the upgrade contract and needs focused
regression coverage.

### 4. Add standalone and inline mutation support

Update `internal/v6provider/resource_security_group_rule.go` and
`internal/v6provider/resource_security_group.go`:

- Project action into `core.SecurityGroupRuleArguments` for standalone and
  inline POST and PATCH requests.
- Include action in `securityGroupRulePropertiesEqual` so an action-only
  standalone edit sends one PATCH.
- Rely on the shared fingerprint and ID transfer logic to distinguish allow and
  deny inline rules with otherwise identical fields.
- Preserve existing IDs and replacement behavior. The working assumption is
  that changing action updates the rule in place.
- After a successful standalone update, store the validated planned action,
  consistent with the existing `ports` and `notes` behavior.
- Keep list order handling unchanged. Do not sort by action.
- Confirm rule reordering remains a state-only operation with no POST, PATCH, or
  DELETE.
- Exercise both plural attributes and deprecated singular blocks. They share
  remote behavior and must retain equivalent state semantics.
- Populate action during standalone and group import, then require a stable
  following plan when configuration matches the imported rules.

### 5. Add focused action acceptance coverage

Add two acceptance lifecycles:

1. `TestAccKatapultSecurityGroupRule_action`
   - Create a standalone rule with action omitted and assert `allow`.
   - Add explicit allow and require an empty plan with the same ID.
   - Change to deny and assert one in-place update with the same ID.
   - Import the deny rule and require stable deny state.
   - Change back to allow and retain the ID.
2. `TestAccKatapultSecurityGroup_rule_actions`
   - Create mixed inline allow and deny rules, including a pair whose other
     fields are equal.
   - Assert each action and capture each ID.
   - Reverse the configured list order and require no remote mutation.
   - Change one action and assert only that rule updates in place.
   - Exercise the preferred plural attributes and one deprecated block step.
   - Read the same rules through the applicable data sources and assert action.

Record and replay `TestAccKatapultSecurityGroupRule_action` first. Its
allow-to-deny step is the decision gate for the generated client's in-place
PATCH contract. If the live API rejects an action-only PATCH, stop before
implementing inline action changes and revise the design to use replacement for
the standalone resource and delete-plus-create reconciliation for inline rules.
Do not silently weaken the same-ID completion gate.

The tests verify provider and API CRUD behavior. They do not test packet flow or
iptables compilation, which belongs to Katapult's firewall implementation.

Live Katapult API recording is authorized for this work. Create or re-record
only these action-affected cassettes, exactly one test per recording command:

```sh
mise exec -- env VCR=rec TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroupRule_action$ -count=1 -parallel=1' \
  make testacc

mise exec -- env VCR=rec TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup_rule_actions$ -count=1 -parallel=1' \
  make testacc

mise exec -- env VCR=rec TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroupRule_example$ -count=1 -parallel=1' \
  make testacc

mise exec -- env VCR=rec TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup_example$ -count=1 -parallel=1' \
  make testacc
```

Replay and inspect each exact cassette before recording or re-recording the
next one. Inspect changes with `git diff --stat`, `git diff --numstat`, `git
diff --name-only`, targeted searches, and bounded ranges. Reject unrelated
cassette, random-ID, or secret drift. Do not batch-re-record the Security Group
suite. Never re-record
`SecurityGroup_migrate_v5_blocks_and_round_trip.cassette.yaml`; it captures
released SDKv2 behavior and its mutation guard makes a mismatch a compatibility
finding, not a recording trigger.

### 6. Update examples and generated documentation

- Add an explicit deny example and an omitted-action allow example to the
  standalone rule resource example.
- Add the deny-SSH-CIDR plus allow-SSH-everywhere example to the preferred
  plural inline rule example.
- Put the default, accepted values, deny-before-allow evaluation, and implicit
  deny-all behavior in the shared `action` attribute description.
- Update the `inbound_rules` and `outbound_rules` descriptions to state that
  list order does not control firewall evaluation.
- Give computed data-source action attributes a matching concise description.
- Do not present the implicit deny-all rule as Terraform state.
- Do not add bespoke Security Group templates for this prose. The schema
  descriptions are sufficient and keep generated resource and data-source docs
  aligned.
- Regenerate all affected resource and data-source pages with `mise run
  docs:generate`. Do not hand-edit generated pages.

## Verification strategy

Use the narrowest evidence while implementing, then broaden before handoff.

### Focused checks

Run the shared model, matcher, schema, equality, normalization, and resource
unit tests during each complete behavioral step. Do not treat the temporary
field-declaration seam in step 1 as a validation boundary. Then run the exact
replay acceptance cases:

```sh
TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroupRule_action$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup_rule_actions$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup_migrate_v5_blocks_and_round_trip$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroupRule_example$' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup_example$' \
  mise run test:acceptance
```

Confirm from test output that each named test ran. Check `git status` after
every acceptance run for cassette and random-ID changes.

### Existing-user regression suite

Run the complete Security Group resource replay group and the separate
data-source group. The resource prefix does not match data-source test names:

```sh
TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultSecurityGroup' \
  mise run test:acceptance

TEST=./internal/v6provider \
  TESTARGS='-run ^TestAccKatapultDataSourceSecurityGroup' \
  mise run test:acceptance
```

This must retain the existing coverage for:

- Genuine protocol-v5 state handover.
- Both inline rule representations and representation round trips.
- Stable rule IDs and reorder behavior.
- Both `external_rules` transitions.
- Allow-all conflicts.
- Standalone-rule coexistence.
- Out-of-band deletion.
- Omitted and explicit optional values.
- Partial inline creation failure.
- Imports and all four data sources.

The handover test is the release blocker. It must prove that adding action does
not force existing users to edit configuration or mutate remote rules. Its
cassette and existing mutation list must remain unchanged.

### Broad checks

In a fresh worktree, run `mise run treeboot` before replay validation. Then run:

```sh
mise run docs:generate
mise run docs:verify
mise run check
mise run verify
git diff --check
```

Review generated documentation changes, the complete source diff, cassette
stats, and any random-ID changes before handoff.

## Review focus

Independent review should concentrate on:

- Existing-state decoding and the first post-upgrade plan.
- Null or missing action in raw SDKv2 state, including canonical behavior when
  refresh is disabled.
- No API mutation during SDKv2 handover or explicit-default adoption.
- Action included in every model, object type, request, response, fingerprint,
  and equality check.
- Distinct ID handling for otherwise equal allow and deny rules.
- No accidental action-based sorting or order-dependent firewall claims.
- Replay compatibility accepting only the default allow omission.
- Matcher compatibility implemented as a value-aware exception rather than a
  generic omittable property.
- Imports and data sources exposing the remote action accurately.
- Generated docs matching the settled firewall semantics.

## Completion gates

- Omitted action is `allow` in plan, request, state, imports, and data sources.
- Null or absent action in released SDKv2 state canonicalizes to `allow` before
  fingerprinting, including with refresh disabled.
- Explicit deny creates and updates successfully through both managed rule
  paths.
- Action-only changes retain the rule ID and issue one PATCH.
- Otherwise equal allow and deny rules retain distinct IDs.
- Reordering rules does not issue remote mutations.
- Genuine SDKv2 standalone and inline state reaches an empty Framework plan
  without ID changes or remote mutations.
- Adding explicit `action = "allow"` after handover produces an empty plan.
- Standalone and inline imports expose normalized action values, including an
  explicit state check for imported group rules.
- Existing Security Group characterization and acceptance tests remain green.
- Both Security Group resource and data-source acceptance groups remain green.
- Replay rejects deny requests against legacy allow-only interactions.
- New recordings contain only the intended interactions and no secrets.
- The genuine migration cassette and its mutation list remain byte-for-byte
  unchanged.
- Generated resource and data-source documentation is current.
- `mise run check`, `mise run verify`, and `git diff --check` pass.
- The worktree contains no unexplained cassette or random-ID drift.

## Rollback and release constraint

Land this work before the next provider release so the first published
Framework Security Group schema already contains action. Before release, a
revert restores the current unreleased schema and behavior. After release,
removing or changing the action field would require an explicit compatibility
and state-migration design.

Do not release if the genuine protocol-v5 handover test, action acceptance
tests, Security Group resource replay group, or Security Group data-source
replay group fails.

## Unresolved questions

None. Cassette recording permission is settled, with the strict requirement to
record or re-record one cassette at a time and verify it before continuing. API
support for action-only PATCH remains live evidence to gather, but the first
standalone action recording is an explicit stop-and-redesign gate if that
assumption fails.
