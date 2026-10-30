# [Terraform](https://www.terraform.io/) [Domain Chief](https://domain.chief.app?ref=terraform-provider-domainchief) provider

[![Release](https://img.shields.io/github/v/release/chieftools/terraform-provider-domainchief)](https://github.com/chieftools/terraform-provider-domainchief/releases)
[![Tests](https://github.com/chieftools/terraform-provider-domainchief/actions/workflows/test.yml/badge.svg)](https://github.com/chieftools/terraform-provider-domainchief/actions/workflows/test.yml)

Manage the hosted DNS records, web redirects and mail forwards of domains in [Domain Chief](https://domain.chief.app?ref=terraform-provider-domainchief) with Terraform or OpenTofu.

## Usage

```terraform
terraform {
  required_providers {
    domainchief = {
      source = "chieftools/domainchief"
    }
  }
}

provider "domainchief" {}

resource "domainchief_dns_record" "www" {
  domain  = "example.com"
  type    = "CNAME"
  name    = "www"
  content = "example.com"
}
```

Set the API token with the `DOMAINCHIEF_API_TOKEN` environment variable. Create a [team access token](https://account.chief.app/team/select/token/create?ref=terraform-provider-domainchief) with only the scopes your configuration needs, the [provider documentation](https://registry.terraform.io/providers/chieftools/domainchief/latest/docs) lists the scope of every resource and data source.

The provider manages existing domains only, it does not register, transfer or delete domains.

## Resources and data sources

| Name | Description |
|---|---|
| `domainchief_dns_record` | A DNS record of a domain using hosted DNS |
| `domainchief_web_redirect` | A web redirect of a domain or subdomain |
| `domainchief_mail_forward` | A mail forward, mail forwarding must already be enabled for the domain |
| `domainchief_domain` (data source) | A domain in the team |
| `domainchief_domains` (data source) | The domains in the team |

## Development

Build the provider:

```bash
make build
```

Run the unit tests and the provider tests, which run Terraform against an in-memory fake of the API:

```bash
make test
```

Run the acceptance tests against the real Domain Chief API. They need a domain using hosted DNS and a token with the `domainchief:dns:write`, `domainchief:redirects_web:write` and `domainchief:domains:read` scopes:

```bash
DOMAINCHIEF_API_TOKEN=ctt_... DOMAINCHIEF_TEST_DOMAIN=example.com make testacc
```

Mail forward acceptance tests also need `DOMAINCHIEF_TEST_MAIL_FORWARDING=1`, a token with the `domainchief:redirects_mail:write` scope and mail forwarding enabled for the domain.

Regenerate the documentation in `docs/` after changing a schema or example:

```bash
make generate
```

## Releasing

Push a `vX.Y.Z` tag. The release workflow builds and signs the provider with GoReleaser and creates a GitHub release, the Terraform Registry picks up new releases automatically.
