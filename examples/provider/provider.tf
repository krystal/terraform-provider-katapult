terraform {
  required_providers {
    katapult = {
      source  = "krystal/katapult"
      version = "~> 0.0"
    }
  }
}

provider "katapult" {
  api_key      = var.api_key
  organization = var.organization
  data_center  = var.data_center
}
