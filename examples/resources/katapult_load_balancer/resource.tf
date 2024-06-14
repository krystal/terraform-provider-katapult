# Directly assign virtual machines to the load balancer
resource "katapult_load_balancer" "by-vms" {
  name = "by-vms"

  virtual_machine_ids = [
    "vm_3HmtE9zPthxuAI6j",
    "vm_ru36Np4eTbXGjTrM"
    
  ]
}

# Assign virtual machines based on groups to the load balancer
resource "katapult_load_balancer" "by-group" {
  name = "by-group"

  virtual_machine_group_ids = [
    "vmgrp_sQx8kjqefpvsLVyu",
    "vmgrp_CICXhD3LrWE5uP46",
    "vmgrp_qaF7p1RqMgSAoybA"
  ]
}


# Assign virtual machines based on tags to the load balancer
resource "katapult_load_balancer" "by-tag" {
  name = "by-tag"

  tag_ids = [
    "tag_2xFkGuXp8iNciPxi",
    "tag_NKWVzB706MdfYODr",
    "tag_SAMo9t0eHM1SuNwX"
  ]
}
