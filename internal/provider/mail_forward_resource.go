package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

var (
	_ resource.Resource                = (*mailForwardResource)(nil)
	_ resource.ResourceWithConfigure   = (*mailForwardResource)(nil)
	_ resource.ResourceWithImportState = (*mailForwardResource)(nil)
)

type mailForwardResource struct {
	client *client.Client
}

type mailForwardModel struct {
	ID     types.String `tfsdk:"id"`
	Domain types.String `tfsdk:"domain"`
	Alias  types.String `tfsdk:"alias"`
	Target types.String `tfsdk:"target"`
}

func newMailForwardResource() resource.Resource {
	return &mailForwardResource{}
}

func (r *mailForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_forward"
}

func (r *mailForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mail forward of a domain that uses Domain Chief hosted DNS.\n\n" +
			"Mail forwarding must already be enabled for the domain, the provider does not enable it because it is a paid feature. " +
			"Requires a token with the `domainchief:redirects_mail:write` scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the mail forward.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "The domain name or domain ID the mail forward belongs to. Changing this creates a new mail forward.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"alias": schema.StringAttribute{
				MarkdownDescription: "The local part to forward, for example `info` for `info@example.com`. Use `*` to forward every address that has no other mail forward.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "The email address that receives the forwarded messages.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(3, 255)},
			},
		},
	}
}

func (r *mailForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	apiClient, err := clientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())

		return
	}

	r.client = apiClient
}

func (r *mailForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailForwardModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	forward, err := r.client.CreateMailForward(ctx, plan.Domain.ValueString(), mailForwardInput(plan))
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to create mail forward", mailForwardError(err))

		return
	}

	applyMailForward(&plan, forward)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mailForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailForwardModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	forward, err := r.client.GetMailForward(ctx, state.Domain.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)

		return
	}

	if err != nil {
		resp.Diagnostics.AddError("Failed to read mail forward", mailForwardError(err))

		return
	}

	applyMailForward(&state, forward)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *mailForwardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailForwardModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(plan.Domain.ValueString())
	forward, err := r.client.UpdateMailForward(ctx, plan.Domain.ValueString(), state.ID.ValueString(), mailForwardInput(plan))
	unlock()

	if err != nil {
		resp.Diagnostics.AddError("Failed to update mail forward", mailForwardError(err))

		return
	}

	applyMailForward(&plan, forward)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mailForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailForwardModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockDomain(state.Domain.ValueString())
	err := r.client.DeleteMailForward(ctx, state.Domain.ValueString(), state.ID.ValueString())
	unlock()

	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete mail forward", mailForwardError(err))
	}
}

func (r *mailForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, id, ok := strings.Cut(req.ID, "/")
	if !ok || domain == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected an import ID in the form <domain>/<mail_forward_id>, for example example.com/01HXYZ.")

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func mailForwardInput(plan mailForwardModel) client.MailForwardInput {
	return client.MailForwardInput{
		Alias:  plan.Alias.ValueString(),
		Target: plan.Target.ValueString(),
	}
}

// applyMailForward copies the API mail forward into the model, keeping configured values that only differ in case.
func applyMailForward(model *mailForwardModel, forward *client.MailForward) {
	model.ID = types.StringValue(forward.ID)

	if !strings.EqualFold(model.Alias.ValueString(), forward.Alias) {
		model.Alias = types.StringValue(forward.Alias)
	}

	if !strings.EqualFold(model.Target.ValueString(), forward.Target) {
		model.Target = types.StringValue(forward.Target)
	}
}

// mailForwardError explains the most common mail forward failure, mail forwarding not being enabled for the domain.
func mailForwardError(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Code == "mail_forwarding.disabled" {
		return "Mail forwarding is not enabled for this domain. Enable it in Domain Chief first, the provider does not enable it because it is a paid feature.\n\n" + err.Error()
	}

	return err.Error()
}
