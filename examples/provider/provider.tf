terraform {
  required_providers {
    domainchief = {
      source = "chieftools/domainchief"
    }
  }
}

# Configure the token with the DOMAINCHIEF_API_TOKEN environment variable.
provider "domainchief" {}
