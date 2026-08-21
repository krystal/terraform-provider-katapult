---
page_title: "Migrating Security Group Rule Syntax"
description: |-
  Move between deprecated security group rule blocks and plural rule attributes without changing remote rules.
---

# Migrating Security Group Rule Syntax

The `inbound_rules` and `outbound_rules` attributes are the preferred way to
manage inline rules on
[`katapult_security_group`](../resources/security_group.md). The singular
`inbound_rule` and `outbound_rule` blocks remain fully functional for backwards
compatibility, but emit a deprecation warning.

Equivalent syntax changes update Terraform state only. They preserve every
rule ID and do not create, update, or delete remote rules.

## Blocks to attributes

Replace each direction's blocks with one list attribute:

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

Run `terraform plan`, review the in-place state-path change, and apply it. Then
run `terraform plan` again and require an empty plan.

## Attributes back to blocks

You can revert configuration to the deprecated blocks when necessary. Replace
the plural attribute for a direction with equivalent blocks, apply the in-place
state-path change, and require the following plan to be empty. Terraform emits
the block deprecation warning again; reverting does not mutate remote rules.

Do not configure both representations for the same direction. Inbound and
outbound directions may migrate independently.
