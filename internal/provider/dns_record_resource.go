package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var (
	_ resource.Resource                   = (*dnsRecordResource)(nil)
	_ resource.ResourceWithConfigure      = (*dnsRecordResource)(nil)
	_ resource.ResourceWithImportState    = (*dnsRecordResource)(nil)
	_ resource.ResourceWithValidateConfig = (*dnsRecordResource)(nil)
)

var recordTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "ALIAS", "CAA", "SRV", "TLSA", "NS"}

type dnsRecordResource struct {
	client *client.Client
}

type dnsRecordModel struct {
	ID       types.String `tfsdk:"id"`
	Domain   types.String `tfsdk:"domain"`
	Type     types.String `tfsdk:"type"`
	Name     types.String `tfsdk:"name"`
	Content  types.String `tfsdk:"content"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Priority types.Int64  `tfsdk:"priority"`
	Comment  types.String `tfsdk:"comment"`
	Metadata types.Map    `tfsdk:"metadata"`
}

func newDNSRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DNS record of a domain that uses Domain Chief hosted DNS.\n\n" +
			"Requires a token with the `domainchief:dns:write` scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the DNS record.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "The domain name or domain ID the record belongs to. Changing this creates a new record.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "The record type, one of `" + strings.Join(recordTypes, "`, `") + "`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(recordTypes...)},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The record name relative to the domain, for example `www`. Use an empty string or `@` for the domain itself. Defaults to the domain itself.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "The record content. TXT content can be written with or without surrounding quotes, and hostnames with or without a trailing dot.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"ttl": schema.Int64Attribute{
				MarkdownDescription: fmt.Sprintf("The record TTL in seconds, one of %s. Defaults to `3600`.", joinInt64s(allowedTTLs)),
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3600),
				Validators:          []validator.Int64{int64validator.OneOf(allowedTTLs...)},
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "The record priority, only for `MX` and `SRV` records. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators:          []validator.Int64{int64validator.Between(0, 65535)},
			},
			"comment": schema.StringAttribute{
				MarkdownDescription: "An optional comment for the record.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"metadata": schema.MapAttribute{
				MarkdownDescription: "Metadata to attach to the record. Keys are at most 40 characters and cannot contain `[` or `]`, values are at most 500 characters. Keys starting with `domainchief:` are reserved for Domain Chief: they cannot be set and the ones Domain Chief adds are not tracked.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators:          metadataValidators(),
			},
		},
	}
}

func (r *dnsRecordResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config dnsRecordModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Type.IsUnknown() || config.Type.IsNull() {
		return
	}

	if !config.Priority.IsNull() && !config.Priority.IsUnknown() && !recordTypesWithPriority[config.Type.ValueString()] {
		resp.Diagnostics.AddAttributeError(
			path.Root("priority"),
			"Priority not supported",
			fmt.Sprintf("A priority can only be set for MX and SRV records, not for %s records.", config.Type.ValueString()),
		)
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	r.client = apiClient
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := recordInput(ctx, plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	record, err := r.client.CreateDNSRecord(ctx, plan.Domain.ValueString(), input)
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())

		return
	}

	resp.Diagnostics.Append(applyDNSRecord(ctx, &plan, record)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	record, err := r.client.GetDNSRecord(ctx, state.Domain.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)

		return
	}

	if err != nil {
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())

		return
	}

	resp.Diagnostics.Append(applyDNSRecord(ctx, &state, record)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsRecordModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := recordInput(ctx, plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	record, err := r.client.UpdateDNSRecord(ctx, plan.Domain.ValueString(), state.ID.ValueString(), input)
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to update DNS record", err.Error())

		return
	}

	plan.ID = state.ID

	resp.Diagnostics.Append(applyDNSRecord(ctx, &plan, record)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(state.Domain.ValueString())
	err := r.client.DeleteDNSRecord(ctx, state.Domain.ValueString(), state.ID.ValueString())
	unlock()

	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, id, ok := strings.Cut(req.ID, "/")
	if !ok || domain == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected an import ID in the form <domain>/<record_id>, for example example.com/01HXYZ.")

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func recordInput(ctx context.Context, plan dnsRecordModel, state *dnsRecordModel) (client.DNSRecordInput, diag.Diagnostics) {
	input := client.DNSRecordInput{
		Type:    plan.Type.ValueString(),
		Name:    plan.Name.ValueString(),
		Content: plan.Content.ValueString(),
		TTL:     plan.TTL.ValueInt64(),
		Comment: plan.Comment.ValueStringPointer(),
	}

	if recordTypesWithPriority[input.Type] {
		priority := plan.Priority.ValueInt64()
		input.Priority = &priority
	}

	desired, diags := stringMap(ctx, plan.Metadata)
	if diags.HasError() {
		return input, diags
	}

	previous := map[string]string{}
	if state != nil {
		var stateDiags diag.Diagnostics
		previous, stateDiags = stringMap(ctx, state.Metadata)
		diags.Append(stateDiags...)
	}

	input.Metadata = metadataChanges(previous, desired)

	return input, diags
}

// applyDNSRecord copies the API record into the model, keeping configured values the API stores in an equivalent form.
func applyDNSRecord(ctx context.Context, model *dnsRecordModel, record *client.DNSRecord) diag.Diagnostics {
	domain := model.Domain.ValueString()

	model.ID = types.StringValue(record.ID)

	if normalizeRecordName(model.Name.ValueString()) != record.Name || model.Name.IsNull() {
		model.Name = types.StringValue(record.Name)
	}

	content := recordContentFromAPI(record)
	if model.Type.ValueString() != record.Type || normalizeRecordContent(record.Type, model.Content.ValueString(), domain) != normalizeRecordContent(record.Type, content, domain) {
		model.Content = types.StringValue(content)
	}

	model.Type = types.StringValue(record.Type)
	model.TTL = types.Int64Value(record.TTL)
	model.Priority = types.Int64Value(record.Priority)

	if record.Comment == nil || *record.Comment == "" {
		model.Comment = types.StringNull()
	} else {
		model.Comment = types.StringValue(*record.Comment)
	}

	return applyMetadata(ctx, &model.Metadata, record.Metadata)
}

// applyMetadata sets the user managed metadata, keeping an unset attribute null when there is no metadata.
func applyMetadata(ctx context.Context, target *types.Map, metadata map[string]string) diag.Diagnostics {
	user := userMetadata(metadata)

	if len(user) == 0 && target.IsNull() {
		return nil
	}

	value, diags := types.MapValueFrom(ctx, types.StringType, user)
	*target = value

	return diags
}

func stringMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	result := map[string]string{}

	if value.IsNull() || value.IsUnknown() {
		return result, nil
	}

	diags := value.ElementsAs(ctx, &result, false)

	return result, diags
}

func metadataValidators() []validator.Map {
	return []validator.Map{
		mapvalidator.KeysAre(
			stringvalidator.LengthBetween(1, 40),
			stringvalidator.RegexMatches(regexp.MustCompile(`^[^\[\]]+$`), "cannot contain [ or ]"),
			noPrefixValidator{prefix: protectedMetadataPrefix},
		),
		mapvalidator.ValueStringsAre(stringvalidator.LengthBetween(1, 500)),
	}
}

func joinInt64s(values []int64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("`%d`", value)
	}

	return strings.Join(parts, ", ")
}
