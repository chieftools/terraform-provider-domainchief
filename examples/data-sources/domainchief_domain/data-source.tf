data "domainchief_domain" "example" {
  domain = "example.com"
}

output "nameservers" {
  value = data.domainchief_domain.example.nameservers[*].hostname
}
