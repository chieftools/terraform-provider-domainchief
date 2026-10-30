package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*dnsRecordsDataSource)(nil)

var dnsRecordAttrTypes = map[string]attr.Type{
	"id":         types.StringType,
	"type":       types.StringType,
	"name":       types.StringType,
	"content":    types.StringType,
	"ttl":        types.Int64Type,
	"priority":   types.Int64Type,
	"comment":    types.StringType,
	"metadata":   types.MapType{ElemType: types.StringType},
	"managed_by": types.StringType,
}

type dnsRecordsDataSource struct {
	client *client.Client
}

type dnsRecordsModel struct {
	Domain  types.String `tfsdk:"domain"`
	Type    types.String `tfsdk:"type"`
	Records types.List   `tfsdk:"records"`
}

func newDNSRecordsDataSource() datasource.DataSource {
	return &dnsRecordsDataSource{}
}

func (d *dnsRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_records"
}

func (d *dnsRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the DNS records of a domain that uses Domain Chief hosted DNS, for example to import an existing zone.\n\n" +
			"Requires a token with the `domainchief:dns:read` scope.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "The domain name or domain ID to list the records of.",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Only return records of this type, one of `" + strings.Join(recordTypes, "`, `") + "`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf(recordTypes...)},
			},
			"records": schema.ListNestedAttribute{
				MarkdownDescription: "The DNS records of the domain.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "The ID of the DNS record."},
						"type":     schema.StringAttribute{Computed: true, MarkdownDescription: "The record type."},
						"name":     schema.StringAttribute{Computed: true, MarkdownDescription: "The record name relative to the domain, empty for the domain itself."},
						"content":  schema.StringAttribute{Computed: true, MarkdownDescription: "The record content, TXT content is returned without surrounding quotes."},
						"ttl":      schema.Int64Attribute{Computed: true, MarkdownDescription: "The record TTL in seconds."},
						"priority": schema.Int64Attribute{Computed: true, MarkdownDescription: "The record priority, `0` for types without a priority."},
						"comment":  schema.StringAttribute{Computed: true, MarkdownDescription: "The comment of the record, if any."},
						"metadata": schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Metadata attached to the record, without the keys managed by Domain Chief."},
						"managed_by": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The Domain Chief feature that manages this record: `web_redirect`, `mail_forwarding` or `acme_challenge`. " +
								"Null for records you manage yourself. Manage these records through their feature, for example with `domainchief_web_redirect`, instead of as a `domainchief_dns_record`. More values may be added in the future.",
						},
					},
				},
			},
		},
	}
}

func (d *dnsRecordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	d.client = apiClient
}

func (d *dnsRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dnsRecordsModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := d.client.ListDNSRecords(ctx, config.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list DNS records", err.Error())

		return
	}

	objects := make([]attr.Value, 0, len(records))

	for i := range records {
		record := &records[i]

		if !config.Type.IsNull() && record.Type != config.Type.ValueString() {
			continue
		}

		metadata, diags := types.MapValueFrom(ctx, types.StringType, userMetadata(record.Metadata))
		resp.Diagnostics.Append(diags...)

		comment := types.StringNull()
		if record.Comment != nil && *record.Comment != "" {
			comment = types.StringValue(*record.Comment)
		}

		object, diags := types.ObjectValue(dnsRecordAttrTypes, map[string]attr.Value{
			"id":         types.StringValue(record.ID),
			"type":       types.StringValue(record.Type),
			"name":       types.StringValue(record.Name),
			"content":    types.StringValue(recordContentFromAPI(record)),
			"ttl":        types.Int64Value(record.TTL),
			"priority":   types.Int64Value(record.Priority),
			"comment":    comment,
			"metadata":   metadata,
			"managed_by": managedBy(record.Metadata),
		})
		resp.Diagnostics.Append(diags...)

		objects = append(objects, object)
	}

	list, diags := types.ListValue(types.ObjectType{AttrTypes: dnsRecordAttrTypes}, objects)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.Records = list
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// managedBy returns the Domain Chief feature that manages a record, based on the metadata Domain Chief adds to it.
func managedBy(metadata map[string]string) types.String {
	if feature := metadata[protectedMetadataPrefix+"managed_feature"]; feature != "" {
		return types.StringValue(feature)
	}

	if metadata[protectedMetadataPrefix+"acme_challenge"] != "" {
		return types.StringValue("acme_challenge")
	}

	return types.StringNull()
}
