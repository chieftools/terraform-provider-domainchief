package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

// Acceptance tests run against the real Domain Chief API and only run with TF_ACC=1.
//
// They need a domain using hosted DNS and a token with the dns, redirects_web and domains:read scopes:
//
//	TF_ACC=1 DOMAINCHIEF_API_TOKEN=ctt_... DOMAINCHIEF_TEST_DOMAIN=example.com go test ./internal/provider/
//
// Mail forward tests also need DOMAINCHIEF_TEST_MAIL_FORWARDING=1 and mail forwarding enabled for the domain.
func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, env := range []string{"DOMAINCHIEF_API_TOKEN", "DOMAINCHIEF_TEST_DOMAIN"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests", env)
		}
	}
}

func testAccDomain() string {
	return os.Getenv("DOMAINCHIEF_TEST_DOMAIN")
}

func testAccClient() *client.Client {
	return client.New(client.Config{
		BaseURL:   os.Getenv("DOMAINCHIEF_API_URL"),
		Token:     os.Getenv("DOMAINCHIEF_API_TOKEN"),
		Team:      os.Getenv("DOMAINCHIEF_TEAM"),
		UserAgent: "terraform-provider-domainchief/acceptance-test",
	})
}

// testAccCheckDestroyed verifies every resource of the given type is gone from the API.
func testAccCheckDestroyed(resourceType string, get func(ctx context.Context, c *client.Client, domain, id string) error) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		c := testAccClient()

		for _, rs := range state.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}

			err := get(context.Background(), c, rs.Primary.Attributes["domain"], rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("%s %s still exists", resourceType, rs.Primary.ID)
			}

			if !client.IsNotFound(err) {
				return err
			}
		}

		return nil
	}
}

func TestAccDNSRecord(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)

	config := func(content, comment string) string {
		return fmt.Sprintf(`
resource "domainchief_dns_record" "txt" {
  domain  = %[1]q
  type    = "TXT"
  name    = %[2]q
  content = %[3]q
  ttl     = 300
  comment = %[4]q
  metadata = {
    source = "terraform-acceptance-test"
  }
}

resource "domainchief_dns_record" "cname" {
  domain  = %[1]q
  type    = "CNAME"
  name    = "%[2]s-alias"
  content = "%[2]s.%[1]s."
}

resource "domainchief_dns_record" "mx" {
  domain   = %[1]q
  type     = "MX"
  name     = %[2]q
  content  = "mail.%[1]s"
  priority = 10
}
`, testAccDomain(), name, content, comment)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testProviderFactories,
		CheckDestroy: testAccCheckDestroyed("domainchief_dns_record", func(ctx context.Context, c *client.Client, domain, id string) error {
			_, err := c.GetDNSRecord(ctx, domain, id)

			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: config("v=spf1 -all", "created"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_dns_record.txt", "content", "v=spf1 -all"),
					resource.TestCheckResourceAttr("domainchief_dns_record.txt", "metadata.source", "terraform-acceptance-test"),
					resource.TestCheckResourceAttr("domainchief_dns_record.cname", "content", name+"."+testAccDomain()+"."),
					resource.TestCheckResourceAttr("domainchief_dns_record.mx", "priority", "10"),
				),
			},
			{
				Config: config("v=spf1 include:example.net -all", "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_dns_record.txt", "content", "v=spf1 include:example.net -all"),
					resource.TestCheckResourceAttr("domainchief_dns_record.txt", "comment", "updated"),
				),
			},
			{
				ResourceName:            "domainchief_dns_record.cname",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       importID("domainchief_dns_record.cname"),
				ImportStateVerifyIgnore: []string{"content"},
			},
		},
	})
}

func TestAccWebRedirect(t *testing.T) {
	subdomain := "tf-acc-" + acctest.RandString(8)

	config := func(statusCode int) string {
		return fmt.Sprintf(`
resource "domainchief_web_redirect" "test" {
  domain       = %q
  subdomain    = %q
  target       = "https://www.example.com/landing"
  status_code  = %d
  include_path = true
}
`, testAccDomain(), subdomain, statusCode)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testProviderFactories,
		CheckDestroy: testAccCheckDestroyed("domainchief_web_redirect", func(ctx context.Context, c *client.Client, domain, id string) error {
			_, err := c.GetWebRedirect(ctx, domain, id)

			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: config(301),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("domainchief_web_redirect.test", "include_www", "false"),
					resource.TestCheckResourceAttr("domainchief_web_redirect.test", "include_query", "true"),
				),
			},
			{
				Config: config(302),
				Check:  resource.TestCheckResourceAttr("domainchief_web_redirect.test", "status_code", "302"),
			},
			{
				ResourceName:      "domainchief_web_redirect.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importID("domainchief_web_redirect.test"),
			},
		},
	})
}

func TestAccMailForward(t *testing.T) {
	if os.Getenv("DOMAINCHIEF_TEST_MAIL_FORWARDING") == "" {
		t.Skip("DOMAINCHIEF_TEST_MAIL_FORWARDING must be set to test mail forwards")
	}

	alias := "tf-acc-" + acctest.RandString(8)

	config := func(target string) string {
		return fmt.Sprintf(`
resource "domainchief_mail_forward" "test" {
  domain = %q
  alias  = %q
  target = %q
}
`, testAccDomain(), alias, target)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testProviderFactories,
		CheckDestroy: testAccCheckDestroyed("domainchief_mail_forward", func(ctx context.Context, c *client.Client, domain, id string) error {
			_, err := c.GetMailForward(ctx, domain, id)

			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: config("terraform-acceptance-test@example.com"),
				Check:  resource.TestCheckResourceAttr("domainchief_mail_forward.test", "alias", alias),
			},
			{
				Config: config("terraform-acceptance-test-updated@example.com"),
				Check:  resource.TestCheckResourceAttr("domainchief_mail_forward.test", "target", "terraform-acceptance-test-updated@example.com"),
			},
			{
				ResourceName:      "domainchief_mail_forward.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importID("domainchief_mail_forward.test"),
			},
		},
	})
}

func TestAccDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "domainchief_domain" "test" {
  domain = %q
}

data "domainchief_domains" "active" {
  status = "active"
}
`, testAccDomain()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.domainchief_domain.test", "is_using_hosted_dns", "true"),
					resource.TestCheckResourceAttrSet("data.domainchief_domain.test", "id"),
					resource.TestCheckResourceAttrSet("data.domainchief_domains.active", "domains.#"),
				),
			},
		},
	})
}
