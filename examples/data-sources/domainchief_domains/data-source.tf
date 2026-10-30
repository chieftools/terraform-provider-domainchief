data "domainchief_domains" "active" {
  status = "active"
}

# Add an SPF record to every active domain that uses hosted DNS.
resource "domainchief_dns_record" "spf" {
  for_each = {
    for domain in data.domainchief_domains.active.domains : domain.domain => domain
    if domain.is_using_hosted_dns
  }

  domain  = each.key
  type    = "TXT"
  content = "v=spf1 -all"
}
