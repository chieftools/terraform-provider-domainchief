# Redirect example.com and www.example.com to another site.
resource "domainchief_web_redirect" "root" {
  domain = "example.com"
  target = "https://www.example.net"
}

# Redirect docs.example.com, keeping the requested path.
resource "domainchief_web_redirect" "docs" {
  domain       = "example.com"
  subdomain    = "docs"
  target       = "https://docs.example.net"
  status_code  = 308
  include_path = true
}
