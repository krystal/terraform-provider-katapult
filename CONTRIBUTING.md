# Contributing

## Provider v5-to-v6 Migration

The provider is being migrated gradually, one resource or data source at a
time. The two implementations are muxed into a single protocol-v6 provider:

- `internal/provider` uses Terraform Plugin SDK v2. It is the remaining legacy
  implementation, exposed as a protocol-v5 server and upgraded to protocol v6
  at runtime.
- `internal/v6provider` uses Terraform Plugin Framework and exposes protocol v6
  natively.

This is intentionally an in-flight migration rather than a flag-day rewrite.
Unless a change specifically migrates an existing type, new resources and data
sources should be implemented only in `internal/v6provider`.

When migrating an existing type:

1. Preserve its public type name and compatible schema/state behavior.
2. Port or add focused replay acceptance coverage in `internal/v6provider`.
3. Remove the corresponding registration from `internal/provider` in the same
   change. A type name cannot be registered by both implementations.
4. Run the focused replay acceptance tests while iterating, then run
   `mise run verify` before handoff.

`TestProviderImplementationsCanBeMuxed` fails if registrations overlap, and
`TestLegacyProviderRegistrations` prevents the legacy allowlist from growing.
The latter should normally only lose entries as the migration progresses.

The `katapult_legacy_*` registrations enabled by `TF_ACC=1` are test-only
fixtures used to compare old and new behavior. They are not public provider
types and do not belong in the production legacy allowlist.
