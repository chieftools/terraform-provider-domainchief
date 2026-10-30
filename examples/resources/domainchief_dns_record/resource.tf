resource "domainchief_dns_record" "www" {
  domain  = "example.com"
  type    = "CNAME"
  name    = "www"
  content = "example.com"
}

resource "domainchief_dns_record" "mx" {
  domain   = "example.com"
  type     = "MX"
  content  = "mail.example.net"
  priority = 10
  ttl      = 3600
}

resource "domainchief_dns_record" "spf" {
  domain  = "example.com"
  type    = "TXT"
  content = "v=spf1 include:example.net -all"
  comment = "Managed by Terraform"

  metadata = {
    owner = "platform"
  }
}
