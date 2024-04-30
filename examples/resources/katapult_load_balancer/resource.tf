# Directly assign virtual machines to the load balancer
resource "katapult_load_balancer" "by-vms" {
  name = "by-vms"

  virtual_machine = [
    {
      id = "vm_3HmtE9zPthxuAI6j"
    },
    {
      id = "vm_ru36Np4eTbXGjTrM"
    }
  ]
}

# Assign virtual machines based on groups to the load balancer
resource "katapult_load_balancer" "by-group" {
  name = "by-group"

  virtual_machine_group = [
    {
      id = "vmgrp_sQx8kjqefpvsLVyu"
    },
    {
      id = "vmgrp_CICXhD3LrWE5uP46"
    },
    {
      id = "vmgrp_qaF7p1RqMgSAoybA"
    }
  ]
}

# Assign virtual machines based on tags to the load balancer
resource "katapult_load_balancer" "by-tag" {
  name = "by-tag"

  tag = [
    {
      id = "tag_2xFkGuXp8iNciPxi"
    },
    {
      id = "tag_NKWVzB706MdfYODr"
    },
    {
      id = "tag_SAMo9t0eHM1SuNwX"
    }
  ]
}
