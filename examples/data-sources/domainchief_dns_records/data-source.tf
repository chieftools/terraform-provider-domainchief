data "domainchief_dns_records" "example" {
  domain = "example.com"
}

# The records you manage yourself, skipping the ones Domain Chief manages for web redirects, mail forwarding and ACME challenges.
output "own_records" {
  value = [
    for record in data.domainchief_dns_records.example.records : "${record.type} ${record.name} ${record.content}"
    if record.managed_by == null
  ]
}
