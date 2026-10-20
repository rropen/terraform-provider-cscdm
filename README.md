<p>
    <img alt="Rolls-Royce Logo" width="100" src="https://raw.githubusercontent.com/rropen/MEC/main/src/frontend/public/logo4.png">
    <br>
    <h3>Terraform Provider for CSC Domain Manager</h3>
</p>
<p>
<a href="https://ghdocs.rollsroyce-sf.com"><img src="https://img.shields.io/badge/Rolls--Royce-Software%20Factory-10069f" alt="sf badge" /></a>
</p>

---

<p>
  <a href="http://commitizen.github.io/cz-cli/"><img src="https://img.shields.io/badge/commitizen-friendly-brightgreen?style=flat" alt="commitizen badge" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/golang-%2300ADD8.svg?style=flat&logo=go&logoColor=white" /></a>
</p>

This Terraform Provider allows managing DNS within CSC Domain Manager.

## Usage Example

```hcl
terraform {
  required_providers {
    cscdm = {
      source = "rropen/cscdm"
    }
  }
}

provider "cscdm" {
    # This provider supports authenticating via the fields below or with
    # the `CSCDM_API_KEY` and `CSCDM_API_TOKEN` environment variables
    api_key = ""
    api_token = ""
}

resource "cscdm_record" "www_example_com" {
  zone  = "example.com"
  type  = "A"
  key   = "www"
  value = "127.0.0.1"
  ttl   = 300

  # optional: sent to CSC as "changeId" and "comments" on the zone edit
  change_id = "example change ID"
  comments  = "some example comments"
}
```

To use the CSC Test environment
```hcl
provider "cscdm" {
    ...

    # This provider supports setting the base url for test environment usage via
    # the field below or with the `CSCDM_BASE_URL` environment variable
    base_url = "https://apis-ote.cscglobal.com/dbs/api/v2/"
}
```

# Terraform imports
Import a record resource. You can get the record id from the error message when you try to create a record that already exists.
```bash
terraform import 'cscdm_record.example["test.domain.com"]' 'domain.com:CNAME:csc_record_id'
```

## Development Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23
- golangci-lint (OSX: brew install golangci-lint)

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

Note: commits must be signed.
https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits

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

When you are ready to raise a pull request..

Resolve any linting issues `make lint`

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

### GOPATH
You may want to adjust your `GOPATH` when building to keep the binary local.

By default, the GOPATH is `$HOME/go`. You can export this to a more local folder if you don't want it dumped in your home folder.

```bash
$ export GOPATH=/path/to/project/
```

### Configure `dev_overrides` for local provider installation

You can configure the default location for the provider binary for Terraform so that any projects using it will load your local development version when running `terraform init`, instead of pulling the provider from the Terraform registry.
See the docs [here](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers).

~/.terraformrc
```hcl
provider_installation {
  dev_overrides {
    "rropen/cscdm" = "/path/to/terraform-provider-cscdm/bin"
  }

  # For all other providers, install as normal.
  direct {}
}
```

