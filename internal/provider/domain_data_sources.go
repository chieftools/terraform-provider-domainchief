package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainsDataSource)(nil)
)

var domainStatuses = []string{"active", "failed", "deleted", "requested", "pending_payment", "scheduled", "restorable"}

var nameserverAttrTypes = map[string]attr.Type{
	"hostname": types.StringType,
	"ipv4":     types.StringType,
	"ipv6":     types.StringType,
}

var domainAttrTypes = map[string]attr.Type{
	"id":                       types.StringType,
	"domain":                   types.StringType,
	"unicode_domain":           types.StringType,
	"status":                   types.StringType,
	"is_locked":                types.BoolType,
	"is_autorenew_enabled":     types.BoolType,
	"is_whois_privacy_enabled": types.BoolType,
	"is_using_hosted_dns":      types.BoolType,
	"is_dnssec_enabled":        types.BoolType,
	"nameservers":              types.ListType{ElemType: types.ObjectType{AttrTypes: nameserverAttrTypes}},
	"renews_at":                types.StringType,
	"expires_at":               types.StringType,
	"mail_forward_status":      types.StringType,
	"metadata":                 types.MapType{ElemType: types.StringType},
}

// domainAttributes describes the computed attributes of a domain, shared by both domain data sources.
func domainAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                       schema.StringAttribute{Computed: true, MarkdownDescription: "The ID of the domain."},
		"unicode_domain":           schema.StringAttribute{Computed: true, MarkdownDescription: "The domain name as Unicode, differs from `domain` only for internationalized domain names."},
		"status":                   schema.StringAttribute{Computed: true, MarkdownDescription: "The status of the domain, for example `active`."},
		"is_locked":                schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the domain is locked against transfers, null when the TLD does not support locking."},
		"is_autorenew_enabled":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the domain renews automatically."},
		"is_whois_privacy_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether WHOIS privacy is enabled, null when the TLD does not support it."},
		"is_using_hosted_dns":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the domain uses Domain Chief hosted DNS. DNS records, web redirects and mail forwards can only be managed for domains using hosted DNS."},
		"is_dnssec_enabled":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether DNSSEC is enabled, null when the TLD does not support it."},
		"nameservers": schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "The nameservers of the domain.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"hostname": schema.StringAttribute{Computed: true, MarkdownDescription: "The hostname of the nameserver."},
					"ipv4":     schema.StringAttribute{Computed: true, MarkdownDescription: "The IPv4 glue address, if any."},
					"ipv6":     schema.StringAttribute{Computed: true, MarkdownDescription: "The IPv6 glue address, if any."},
				},
			},
		},
		"renews_at":           schema.StringAttribute{Computed: true, MarkdownDescription: "When the domain renews, null when auto-renewal is disabled."},
		"expires_at":          schema.StringAttribute{Computed: true, MarkdownDescription: "When the domain expires if it is not renewed."},
		"mail_forward_status": schema.StringAttribute{Computed: true, MarkdownDescription: "The mail forwarding status, mail forwards can only be managed when this is not `disabled`."},
		"metadata":            schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Metadata attached to the domain."},
	}
}

func domainObject(ctx context.Context, domain *client.Domain) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	nameservers := make([]attr.Value, 0, len(domain.Nameservers))
	for _, nameserver := range domain.Nameservers {
		value, d := types.ObjectValue(nameserverAttrTypes, map[string]attr.Value{
			"hostname": types.StringValue(nameserver.Hostname),
			"ipv4":     types.StringPointerValue(nameserver.IPv4),
			"ipv6":     types.StringPointerValue(nameserver.IPv6),
		})
		diags.Append(d...)
		nameservers = append(nameservers, value)
	}

	nameserverList, d := types.ListValue(types.ObjectType{AttrTypes: nameserverAttrTypes}, nameservers)
	diags.Append(d...)

	metadata, d := types.MapValueFrom(ctx, types.StringType, nonNilMap(domain.Metadata))
	diags.Append(d...)

	object, d := types.ObjectValue(domainAttrTypes, map[string]attr.Value{
		"id":                       types.StringValue(domain.ID),
		"domain":                   types.StringValue(domain.Domain),
		"unicode_domain":           types.StringValue(domain.UnicodeDomain),
		"status":                   types.StringValue(domain.Status),
		"is_locked":                types.BoolPointerValue(domain.IsLocked),
		"is_autorenew_enabled":     types.BoolValue(domain.IsAutorenewEnabled),
		"is_whois_privacy_enabled": types.BoolPointerValue(domain.IsWhoisPrivacyEnabled),
		"is_using_hosted_dns":      types.BoolValue(domain.IsUsingHostedDNS),
		"is_dnssec_enabled":        types.BoolPointerValue(domain.IsDNSSECEnabled),
		"nameservers":              nameserverList,
		"renews_at":                types.StringPointerValue(domain.RenewsAt),
		"expires_at":               types.StringPointerValue(domain.ExpiresAt),
		"mail_forward_status":      types.StringValue(domain.MailForwardStatus),
		"metadata":                 metadata,
	})
	diags.Append(d...)

	return object, diags
}

func nonNilMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}

	return values
}

type domainDataSource struct {
	client *client.Client
}

func newDomainDataSource() datasource.DataSource {
	return &domainDataSource{}
}

func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := domainAttributes()
	attributes["domain"] = schema.StringAttribute{
		MarkdownDescription: "The domain name or domain ID to look up.",
		Required:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a domain in the team.\n\nRequires a token with the `domainchief:domains:read` scope.",
		Attributes:          attributes,
	}
}

func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	d.client = apiClient
}

func (d *domainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var lookup types.String

	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("domain"), &lookup)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := d.client.GetDomain(ctx, lookup.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read domain", err.Error())

		return
	}

	// Keep the configured lookup value, which may be the domain ID instead of its name.
	domain.Domain = lookup.ValueString()

	object, diags := domainObject(ctx, domain)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, object)...)
}

type domainsDataSource struct {
	client *client.Client
}

type domainsModel struct {
	Query   types.String `tfsdk:"query"`
	Status  types.String `tfsdk:"status"`
	Domains types.List   `tfsdk:"domains"`
}

func newDomainsDataSource() datasource.DataSource {
	return &domainsDataSource{}
}

func (d *domainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domains"
}

func (d *domainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := domainAttributes()
	attributes["domain"] = schema.StringAttribute{Computed: true, MarkdownDescription: "The domain name."}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the domains in the team.\n\nRequires a token with the `domainchief:domains:read` scope.",
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				MarkdownDescription: "Only return domains matching this search query.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Only return domains with this status, one of `active`, `failed`, `deleted`, `requested`, `pending_payment`, `scheduled` or `restorable`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf(domainStatuses...)},
			},
			"domains": schema.ListNestedAttribute{
				MarkdownDescription: "The matching domains.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: attributes},
			},
		},
	}
}

func (d *domainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	d.client = apiClient
}

func (d *domainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config domainsModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domains, err := d.client.ListDomains(ctx, config.Query.ValueString(), config.Status.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list domains", err.Error())

		return
	}

	objects := make([]attr.Value, 0, len(domains))
	for i := range domains {
		object, diags := domainObject(ctx, &domains[i])
		resp.Diagnostics.Append(diags...)
		objects = append(objects, object)
	}

	list, diags := types.ListValue(types.ObjectType{AttrTypes: domainAttrTypes}, objects)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Domains = list
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
