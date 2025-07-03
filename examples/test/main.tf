terraform {
  required_providers {
    cscdm = {
      source = "registry.terraform.io/rolls-royce/csc-domain-manager"
    }
  }
}

provider "cscdm" {}

locals {
  zone = "rollsroyce-sf.com"
}

resource "cscdm_record" "tf-test-0_rollsroyce-sf_com" {
  zone_name = local.zone
  type      = "CNAME"
  key       = "tf-test-0"
  value     = "devyellowk8s"
  ttl       = 300
}

resource "cscdm_record" "tf-test-1_rollsroyce-sf_com" {
  zone_name = local.zone
  type      = "CNAME"
  key       = "tf-test-1"
  value     = "devyellowk8s"
  ttl       = 300
}

resource "cscdm_record" "tf-test-2_rollsroyce-sf_com" {
  zone_name = local.zone
  type      = "CNAME"
  key       = "tf-test-2"
  value     = "devyellowk8s"
  ttl       = 300
}

resource "cscdm_record" "tf-test-3_rollsroyce-sf_com" {
  zone_name = local.zone
  type      = "CNAME"
  key       = "tf-test-3"
  value     = "devyellowk8s"
  ttl       = 300
}

resource "cscdm_record" "tf-test-x_rollsroyce-sf_com" {
  for_each = toset([for num in range(4, 42) : tostring(num)])

  zone_name = local.zone
  type      = "CNAME"
  key       = "tf-test-${each.key}"
  value     = "devyellowk8s"
  ttl       = 300
}

data "cscdm_zones" "all" {
  depends_on = [
    cscdm_record.tf-test-0_rollsroyce-sf_com,
    cscdm_record.tf-test-1_rollsroyce-sf_com,
    cscdm_record.tf-test-2_rollsroyce-sf_com,
    cscdm_record.tf-test-3_rollsroyce-sf_com,
    cscdm_record.tf-test-x_rollsroyce-sf_com,
  ]
}

output "total_zones" {
  value = length(data.cscdm_zones.all.zones)
}

output "total_cname_records" {
  value = length(flatten(data.cscdm_zones.all.zones[*].cname))
}

output "test_cname_records" {
  value = [for record in flatten(data.cscdm_zones.all.zones[*].cname) : record if startswith(record.key, "tf-test-")]
}

output "resource_test_cname_records" {
  value = cscdm_record.tf-test-x_rollsroyce-sf_com
}
