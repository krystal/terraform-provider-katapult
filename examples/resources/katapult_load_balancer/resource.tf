# Directly assign virtual machines to the laod balancer
resource "katapult_load_balancer" "by-vms" {
  name = "by-vms"

  virtual_machine {
    id = "vm_3HmtE9zPthxuAI6j"
  }

  virtual_machine {
    id = "vm_ru36Np4eTbXGjTrM"
  }
}

# Same as above, but dynamically specify virtual_machine blocks.
resource "katapult_load_balancer" "dynamic-block" {
  name = "dynamic-block"

  dynamic "virtual_machine" {
    for_each = ["vm_3HmtE9zPthxuAI6j", "vm_ru36Np4eTbXGjTrM"]

    content {
      id = virtual_machine.value
    }
  }
}

# Assign virtual machines based on groups to the laod balancer
resource "katapult_load_balancer" "by-group" {
  name = "by-group"

  virtual_machine_group {
    id = "vmgrp_sQx8kjqefpvsLVyu"
  }

  dynamic "virtual_machine_group" {
    for_each = ["vmgrp_CICXhD3LrWE5uP46", "vmgrp_qaF7p1RqMgSAoybA"]

    content {
      id = virtual_machine_group.value
    }
  }
}

# Assign virtual machines based on tags to the laod balancer
resource "katapult_load_balancer" "by-tag" {
  name = "by-tag"

  tag {
    id = "tag_2xFkGuXp8iNciPxi"
  }

  dynamic "virtual_machine_group" {
    for_each = ["tag_NKWVzB706MdfYODr", "tag_SAMo9t0eHM1SuNwX"]

    content {
      id = tag.value
    }
  }
}
