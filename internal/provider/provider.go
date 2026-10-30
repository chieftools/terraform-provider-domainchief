// Package provider implements the Domain Chief Terraform provider.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var _ provider.Provider = (*domainChiefProvider)(nil)

type domainChiefProvider struct {
	version string
}

type providerModel struct {
	APIToken types.String `tfsdk:"api_token"`
	APIURL   types.String `tfsdk:"api_url"`
	Team     types.String `tfsdk:"team"`
}

// New returns a provider factory for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &domainChiefProvider{version: version}
	}
}

func (p *domainChiefProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "domainchief"
	resp.Version = p.version
}

func (p *domainChiefProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage hosted DNS records, web redirects and mail forwards of domains in [Domain Chief](https://domain.chief.app).",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "A Domain Chief API token. Use a team access token with only the scopes your configuration needs. Can also be set with the `DOMAINCHIEF_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"team": schema.StringAttribute{
				MarkdownDescription: "The ID of the team to manage, only needed for tokens that are not scoped to a single team, such as personal access tokens. Can also be set with the `DOMAINCHIEF_TEAM` environment variable.",
				Optional:            true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "The base URL of the Domain Chief API. Defaults to `" + client.DefaultBaseURL + "`. Can also be set with the `DOMAINCHIEF_API_URL` environment variable.",
				Optional:            true,
			},
		},
	}
}

func (p *domainChiefProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIToken.IsUnknown() || config.Team.IsUnknown() || config.APIURL.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown Domain Chief provider configuration",
			"The provider configuration depends on values that are not known until apply. Use static values or environment variables instead.",
		)

		return
	}

	token := valueOrEnv(config.APIToken, "DOMAINCHIEF_API_TOKEN")
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing Domain Chief API token",
			"Set the api_token provider argument or the DOMAINCHIEF_API_TOKEN environment variable.",
		)

		return
	}

	apiClient := client.New(client.Config{
		BaseURL:   valueOrEnv(config.APIURL, "DOMAINCHIEF_API_URL"),
		Token:     token,
		Team:      valueOrEnv(config.Team, "DOMAINCHIEF_TEAM"),
		UserAgent: fmt.Sprintf("terraform-provider-domainchief/%s (Terraform %s)", p.version, req.TerraformVersion),
	})

	resp.DataSourceData = apiClient
	resp.ResourceData = apiClient
}

func (p *domainChiefProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newDNSRecordResource,
		newWebRedirectResource,
		newMailForwardResource,
	}
}

func (p *domainChiefProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newDomainDataSource,
		newDomainsDataSource,
		newDNSRecordsDataSource,
	}
}

func valueOrEnv(value types.String, env string) string {
	if !value.IsNull() && value.ValueString() != "" {
		return value.ValueString()
	}

	return os.Getenv(env)
}

// clientFromProviderData extracts the API client configured by the provider.
func clientFromProviderData(data any) (*client.Client, error) {
	if data == nil {
		return nil, nil
	}

	apiClient, ok := data.(*client.Client)
	if !ok {
		return nil, fmt.Errorf("expected *client.Client, got %T", data)
	}

	return apiClient, nil
}
