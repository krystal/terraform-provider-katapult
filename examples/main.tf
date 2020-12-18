terraform {
  required_providers {
    katapult = {
      version = "0.0.1"
      source  = "katapult.io/katapult/katapult"
    }
  }
}

provider "katapult" {}

data "katapult_data_center" "netwise" {
  permalink = "netwise"
}

resource "katapult_load_balancer" "tf-test" {
  data_center_id = data.katapult_data_center.netwise.id
  name           = "tf-test-2"

  virtual_machine {
    id = "vm_HvHlc0mnvMBkwyev"
  }
}

output "lb_data" {
  value = katapult_load_balancer.tf-test
}
