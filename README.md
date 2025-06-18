# Terraform Provider for CSC Domain Manager
This Terraform Provider allows managing DNS within CSC Domain Manager.

## Usage Example
```hcl
terraform {
  required_providers {
    cscdm = {
      source = "registry.terraform.io/rolls-royce/csc-domain-manager"
    }
  }
}

provider "cscdm" {
    # This provider supports authenticating via the fields below or with
    # the `CSCDM_API_KEY` and `CSCDM_API_TOKEN` environment variables
    api_key = ""
    api_token = ""
}

data "cscdm_zones" "all" {}

output "all_zones" {
  value = data.cscdm_zones.all
}
```

## Development Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
