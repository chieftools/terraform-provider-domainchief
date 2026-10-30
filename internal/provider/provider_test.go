package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var testProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"domainchief": providerserver.NewProtocol6WithError(New("test")()),
}

func fakeProviderConfig(url string) string {
	return fmt.Sprintf(`
provider "domainchief" {
  api_token = "test-token"
  api_url   = %q
}
`, url)
}

func TestDNSRecordKeepsEquivalentConfiguredValues(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "spf" {
  domain  = "example.com"
  type    = "TXT"
  name    = "@"
  content = "v=spf1 -all"
}

resource "domainchief_dns_record" "quoted" {
  domain  = "example.com"
  type    = "TXT"
  name    = "Quoted"
  content = "\"quoted value\""
}

resource "domainchief_dns_record" "alias" {
  domain  = "example.com"
  type    = "CNAME"
  name    = "www."
  content = "target.example.net."
}

resource "domainchief_dns_record" "apex_target" {
  domain  = "example.com"
  type    = "CNAME"
  name    = "app"
  content = "@"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_dns_record.spf", "name", "@"),
					resource.TestCheckResourceAttr("domainchief_dns_record.spf", "content", "v=spf1 -all"),
					resource.TestCheckResourceAttr("domainchief_dns_record.spf", "ttl", "3600"),
					resource.TestCheckResourceAttr("domainchief_dns_record.quoted", "content", `"quoted value"`),
					resource.TestCheckResourceAttr("domainchief_dns_record.alias", "name", "www."),
					resource.TestCheckResourceAttr("domainchief_dns_record.alias", "content", "target.example.net."),
					resource.TestCheckResourceAttr("domainchief_dns_record.apex_target", "content", "@"),
					resource.TestCheckNoResourceAttr("domainchief_dns_record.spf", "metadata.%"),
				),
			},
		},
	})
}

func TestDNSRecordUpdatesMetadataAndPriority(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "mx" {
  domain   = "example.com"
  type     = "MX"
  content  = "mail.example.net"
  priority = 10
  ttl      = 300
  comment  = "primary"
  metadata = {
    owner = "team-a"
    env   = "prod"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "priority", "10"),
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "metadata.%", "2"),
				),
			},
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "mx" {
  domain   = "example.com"
  type     = "MX"
  content  = "mail.example.net"
  priority = 20
  ttl      = 300
  metadata = {
    owner = "team-b"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "priority", "20"),
					resource.TestCheckNoResourceAttr("domainchief_dns_record.mx", "comment"),
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "metadata.%", "1"),
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "metadata.owner", "team-b"),
				),
			},
			{
				ResourceName:      "domainchief_dns_record.mx",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importID("domainchief_dns_record.mx"),
			},
		},
	})
}

func TestDNSRecordRejectsInvalidConfiguration(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "a" {
  domain   = "example.com"
  type     = "A"
  content  = "192.0.2.1"
  priority = 10
}
`,
				ExpectError: regexp.MustCompile(`A priority can only be set for MX and SRV records`),
			},
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "a" {
  domain  = "example.com"
  type    = "A"
  content = "192.0.2.1"
  ttl     = 120
}
`,
				ExpectError: regexp.MustCompile(`Attribute ttl value must be one of`),
			},
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_dns_record" "a" {
  domain   = "example.com"
  type     = "A"
  content  = "192.0.2.1"
  metadata = {
    "domainchief:managed" = "false"
  }
}
`,
				ExpectError: regexp.MustCompile(`Reserved metadata key`),
			},
		},
	})
}

func TestDNSRecordIsRecreatedAfterDeletionOutsideTerraform(t *testing.T) {
	api, url := newFakeAPI(t)

	config := fakeProviderConfig(url) + `
resource "domainchief_dns_record" "a" {
  domain  = "example.com"
  type    = "A"
  name    = "deleted"
  content = "192.0.2.1"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					api.mu.Lock()
					clear(api.records)
					api.mu.Unlock()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestWebRedirectDefaultsAndUpdates(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_web_redirect" "root" {
  domain = "example.com"
  target = "https://www.example.net"
}

resource "domainchief_web_redirect" "docs" {
  domain    = "example.com"
  subdomain = "Docs"
  target    = "https://docs.example.net"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "status_code", "301"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "include_www", "true"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "include_path", "false"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "include_query", "true"),
					resource.TestCheckNoResourceAttr("domainchief_web_redirect.root", "subdomain"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.docs", "subdomain", "Docs"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.docs", "include_www", "false"),
				),
			},
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_web_redirect" "root" {
  domain       = "example.com"
  target       = "https://www.example.org"
  status_code  = 308
  include_www  = false
  include_path = true
}

resource "domainchief_web_redirect" "docs" {
  domain    = "example.com"
  subdomain = "Docs"
  target    = "https://docs.example.net"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "target", "https://www.example.org"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "status_code", "308"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.root", "include_www", "false"),
				),
			},
			{
				ResourceName:      "domainchief_web_redirect.root",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importID("domainchief_web_redirect.root"),
			},
		},
	})
}

func TestWebRedirectRejectsIncludeWWWForSubdomain(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_web_redirect" "docs" {
  domain      = "example.com"
  subdomain   = "docs"
  target      = "https://docs.example.net"
  include_www = true
}
`,
				ExpectError: regexp.MustCompile(`include_www only applies to a redirect of the domain itself`),
			},
		},
	})
}

func TestMailForwardKeepsConfiguredCase(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_mail_forward" "info" {
  domain = "example.com"
  alias  = "Info"
  target = "inbox@example.net"
}
`,
				Check: resource.TestCheckResourceAttr("domainchief_mail_forward.info", "alias", "Info"),
			},
			{
				ResourceName:            "domainchief_mail_forward.info",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       importID("domainchief_mail_forward.info"),
				ImportStateVerifyIgnore: []string{"alias"},
			},
		},
	})
}

func TestMailForwardExplainsDisabledMailForwarding(t *testing.T) {
	api, url := newFakeAPI(t)
	api.mailForwardingDisabled = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
resource "domainchief_mail_forward" "info" {
  domain = "example.com"
  alias  = "info"
  target = "inbox@example.net"
}
`,
				ExpectError: regexp.MustCompile(`Mail forwarding is not enabled for this domain`),
			},
		},
	})
}

func TestDataSources(t *testing.T) {
	_, url := newFakeAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeProviderConfig(url) + `
data "domainchief_domain" "by_id" {
  domain = "01DOMAIN"
}

data "domainchief_domains" "all" {
  status = "active"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.domainchief_domain.by_id", "domain", "01DOMAIN"),
					resource.TestCheckResourceAttr("data.domainchief_domain.by_id", "unicode_domain", fakeDomain),
					resource.TestCheckResourceAttr("data.domainchief_domain.by_id", "is_using_hosted_dns", "true"),
					resource.TestCheckNoResourceAttr("data.domainchief_domain.by_id", "is_whois_privacy_enabled"),
					resource.TestCheckResourceAttr("data.domainchief_domain.by_id", "nameservers.0.hostname", "ns.domainchief.eu"),
					resource.TestCheckResourceAttr("data.domainchief_domain.by_id", "metadata.customer", "acme"),
					resource.TestCheckResourceAttr("data.domainchief_domains.all", "domains.#", "1"),
					resource.TestCheckResourceAttr("data.domainchief_domains.all", "domains.0.domain", fakeDomain),
				),
			},
		},
	})
}

func TestProviderRequiresAPIToken(t *testing.T) {
	t.Setenv("DOMAINCHIEF_API_TOKEN", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "domainchief" {}

data "domainchief_domain" "example" {
  domain = "example.com"
}
`,
				ExpectError: regexp.MustCompile(`Missing Domain Chief API token`),
			},
		},
	})
}

// importID builds the <domain>/<id> import ID of a resource in state.
func importID(name string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		rs, ok := state.RootModule().Resources[name]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", name)
		}

		return rs.Primary.Attributes["domain"] + "/" + rs.Primary.ID, nil
	}
}
