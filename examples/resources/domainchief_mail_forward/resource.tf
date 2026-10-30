# Mail forwarding must already be enabled for the domain.
resource "domainchief_mail_forward" "info" {
  domain = "example.com"
  alias  = "info"
  target = "team@example.net"
}

# Forward every address without its own mail forward.
resource "domainchief_mail_forward" "catch_all" {
  domain = "example.com"
  alias  = "*"
  target = "inbox@example.net"
}
