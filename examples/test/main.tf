# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    cscdm = {
      source = "registry.terraform.io/rolls-royce/csc-domain-manager"
    }
  }
}

provider "cscdm" {}

data "cscdm_zones" "all" {}

output "all_zones" {
  value = data.cscdm_zones.all
}
