terraform {
  required_providers {
    cscdm = {
      source = "registry.terraform.io/rolls-royce/csc-domain-manager"
    }
  }
}

provider "cscdm" {}

data "cscdm_zones" "all" {}

resource "cscdm_record" "tf-test-0_rollsroyce-sf_com" {
  zone_name = "rollsroyce-sf.com"
  type      = "CNAME"
  key       = "tf-test-0"
  value     = "devyellowk8s"
  ttl       = 300
}

output "total_zones" {
  value = length(data.cscdm_zones.all)
}

output "test_record" {
  value = format(
    "%s: %s",
    resource.cscdm_record.tf-test-0_rollsroyce-sf_com.key,
    resource.cscdm_record.tf-test-0_rollsroyce-sf_com.value,
  )
}
