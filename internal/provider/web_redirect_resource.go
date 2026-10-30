package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var (
	_ resource.Resource                   = (*webRedirectResource)(nil)
	_ resource.ResourceWithConfigure      = (*webRedirectResource)(nil)
	_ resource.ResourceWithImportState    = (*webRedirectResource)(nil)
	_ resource.ResourceWithValidateConfig = (*webRedirectResource)(nil)
)

type webRedirectResource struct {
	client *client.Client
}

type webRedirectModel struct {
	ID           types.String `tfsdk:"id"`
	Domain       types.String `tfsdk:"domain"`
	Subdomain    types.String `tfsdk:"subdomain"`
	Target       types.String `tfsdk:"target"`
	StatusCode   types.Int64  `tfsdk:"status_code"`
	IncludeWWW   types.Bool   `tfsdk:"include_www"`
	IncludePath  types.Bool   `tfsdk:"include_path"`
	IncludeQuery types.Bool   `tfsdk:"include_query"`
}

func newWebRedirectResource() resource.Resource {
	return &webRedirectResource{}
}

func (r *webRedirectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_web_redirect"
}

func (r *webRedirectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a web redirect of a domain that uses Domain Chief hosted DNS. Domain Chief creates the DNS records the redirect needs.\n\n" +
			"Requires a token with the `domainchief:redirects_web:write` scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the web redirect.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "The domain name or domain ID the redirect belongs to. Changing this creates a new redirect.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"subdomain": schema.StringAttribute{
				MarkdownDescription: "The subdomain to redirect, for example `docs`. Leave unset to redirect the domain itself. Changing this creates a new redirect.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "The URL to redirect to, starting with `http://` or `https://`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(httpURLPattern, "must start with http:// or https://")},
			},
			"status_code": schema.Int64Attribute{
				MarkdownDescription: "The HTTP status code of the redirect, one of `301`, `302`, `307` or `308`. Defaults to `301`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(301),
				Validators:          []validator.Int64{int64validator.OneOf(301, 302, 307, 308)},
			},
			"include_www": schema.BoolAttribute{
				MarkdownDescription: "Whether the `www` subdomain is redirected together with the domain itself. Only applies when `subdomain` is unset. Defaults to `true` for the domain itself and `false` for a subdomain.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{includeWWWDefault{}},
			},
			"include_path": schema.BoolAttribute{
				MarkdownDescription: "Whether the requested path is appended to the target URL. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"include_query": schema.BoolAttribute{
				MarkdownDescription: "Whether the requested query string is appended to the target URL. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *webRedirectResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config webRedirectModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.Subdomain.IsNull() && !config.IncludeWWW.IsNull() && !config.IncludeWWW.IsUnknown() && config.IncludeWWW.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("include_www"),
			"include_www not supported for subdomains",
			"include_www only applies to a redirect of the domain itself, remove it or set it to false when subdomain is set.",
		)
	}
}

func (r *webRedirectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	r.client = apiClient
}

func (r *webRedirectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webRedirectModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	redirect, err := r.client.CreateWebRedirect(ctx, plan.Domain.ValueString(), webRedirectInput(plan))
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to create web redirect", err.Error())

		return
	}

	applyWebRedirect(&plan, redirect)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webRedirectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webRedirectModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	redirect, err := r.client.GetWebRedirect(ctx, state.Domain.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)

		return
	}

	if err != nil {
		resp.Diagnostics.AddError("Failed to read web redirect", err.Error())

		return
	}

	applyWebRedirect(&state, redirect)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *webRedirectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state webRedirectModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	redirect, err := r.client.UpdateWebRedirect(ctx, plan.Domain.ValueString(), state.ID.ValueString(), webRedirectInput(plan))
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to update web redirect", err.Error())

		return
	}

	applyWebRedirect(&plan, redirect)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webRedirectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webRedirectModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(state.Domain.ValueString())
	err := r.client.DeleteWebRedirect(ctx, state.Domain.ValueString(), state.ID.ValueString())
	unlock()

	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete web redirect", err.Error())
	}
}

func (r *webRedirectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, id, ok := strings.Cut(req.ID, "/")
	if !ok || domain == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected an import ID in the form <domain>/<redirect_id>, for example example.com/01HXYZ.")

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func webRedirectInput(plan webRedirectModel) client.WebRedirectInput {
	return client.WebRedirectInput{
		Subdomain:    plan.Subdomain.ValueStringPointer(),
		IncludeWWW:   plan.IncludeWWW.ValueBool(),
		StatusCode:   plan.StatusCode.ValueInt64(),
		Target:       plan.Target.ValueString(),
		IncludePath:  plan.IncludePath.ValueBool(),
		IncludeQuery: plan.IncludeQuery.ValueBool(),
	}
}

// applyWebRedirect copies the API redirect into the model, keeping a configured subdomain the API stores lowercased.
func applyWebRedirect(model *webRedirectModel, redirect *client.WebRedirect) {
	model.ID = types.StringValue(redirect.ID)

	switch {
	case redirect.Subdomain == nil:
		model.Subdomain = types.StringNull()
	case strings.ToLower(strings.TrimSpace(model.Subdomain.ValueString())) != *redirect.Subdomain:
		model.Subdomain = types.StringValue(*redirect.Subdomain)
	}

	model.Target = types.StringValue(redirect.Target)
	model.StatusCode = types.Int64Value(redirect.StatusCode)
	model.IncludeWWW = types.BoolValue(redirect.IncludeWWW)
	model.IncludePath = types.BoolValue(redirect.IncludePath)
	model.IncludeQuery = types.BoolValue(redirect.IncludeQuery)
}

// includeWWWDefault defaults include_www to true for a redirect of the domain itself and to false for a subdomain.
type includeWWWDefault struct{}

var _ planmodifier.Bool = includeWWWDefault{}

func (m includeWWWDefault) Description(_ context.Context) string {
	return "Defaults to true without a subdomain and to false with a subdomain."
}

func (m includeWWWDefault) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m includeWWWDefault) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.ConfigValue.IsNull() {
		return
	}

	var subdomain types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("subdomain"), &subdomain)...)

	if subdomain.IsUnknown() {
		return
	}

	resp.PlanValue = types.BoolValue(subdomain.IsNull())
}
