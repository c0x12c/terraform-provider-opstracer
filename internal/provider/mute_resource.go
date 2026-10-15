package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &muteResource{}
	_ resource.ResourceWithConfigure   = &muteResource{}
	_ resource.ResourceWithImportState = &muteResource{}
)

type muteResource struct {
	client *client.Client
}

type muteModel struct {
	ID        types.String `tfsdk:"id"`
	ScopeType types.String `tfsdk:"scope_type"`
	ScopeID   types.String `tfsdk:"scope_id"`
	StartsAt  types.String `tfsdk:"starts_at"`
	EndsAt    types.String `tfsdk:"ends_at"`
	Reason    types.String `tfsdk:"reason"`
	ServiceID types.String `tfsdk:"service_id"`
	Active    types.Bool   `tfsdk:"active"`
}

func NewMuteResource() resource.Resource {
	return &muteResource{}
}

func (r *muteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mute"
}

func (r *muteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Silences every alert of a service or an integration for a time window, like a maintenance " +
			"window. To silence only some alerts of a service, use `opstracer_suppression`. A mute cannot be " +
			"edited, so any change replaces it, except writing the same time in another UTC offset. Once the " +
			"window is over the mute stays in state as inactive; " +
			"a mute ended early outside Terraform is created again. Destroy ends a running mute and removes a " +
			"scheduled one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Mute ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"scope_type": schema.StringAttribute{
				Description:   "What to mute: `service` or `integration`.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf("service", "integration")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"scope_id": schema.StringAttribute{
				Description:   "ID of the service or integration to mute.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"starts_at": schema.StringAttribute{
				Description: "RFC 3339 start of the window, e.g. `2026-10-20T09:00:00+07:00`. Defaults to the time of creation.",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{rfc3339Validator{}},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					replaceUnlessSameInstant(),
				},
			},
			"ends_at": schema.StringAttribute{
				Description:   "RFC 3339 end of the window. Must be in the future and after `starts_at`.",
				Required:      true,
				Validators:    []validator.String{rfc3339Validator{}},
				PlanModifiers: []planmodifier.String{replaceUnlessSameInstant()},
			},
			"reason": schema.StringAttribute{
				Description:   "Why alerts are muted.",
				Optional:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"service_id": schema.StringAttribute{
				Description:   "Service the mute applies to.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"active": schema.BoolAttribute{
				Description:   "Whether the mute is silencing alerts right now. False before the start and after the end.",
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *muteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *muteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan muteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.client.CreateMute(ctx, client.MuteInput{
		ScopeType: plan.ScopeType.ValueString(),
		ScopeID:   plan.ScopeID.ValueString(),
		StartsAt:  knownString(plan.StartsAt),
		EndsAt:    plan.EndsAt.ValueString(),
		Reason:    plan.Reason.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating mute", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, muteFromAPI(m, plan))...)
}

// There is no get-by-id endpoint, so Read scans the list, which only has scheduled and running mutes.
func (r *muteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state muteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutes, err := r.client.ListMutes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading mutes", err.Error())
		return
	}
	for _, m := range mutes {
		if m.ID == state.ID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, muteFromAPI(&m, state))...)
			return
		}
	}
	if windowOver(state.EndsAt) {
		state.Active = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update only runs when a time is rewritten as the same instant (e.g. after import), so nothing is sent.
func (r *muteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state muteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.StartsAt, state.EndsAt = plan.StartsAt, plan.EndsAt
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *muteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state muteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.EndMute(ctx, state.ID.ValueString())
	if client.ErrorCode(err) == "mute_not_started" {
		err = r.client.DeleteMute(ctx, state.ID.ValueString())
	}
	if err != nil && !client.IsNotFound(err) && client.ErrorCode(err) != "mute_already_ended" {
		resp.Diagnostics.AddError("Error ending mute", err.Error())
	}
}

func (r *muteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func muteFromAPI(m *client.Mute, prior muteModel) muteModel {
	return muteModel{
		ID:        types.StringValue(m.ID),
		ScopeType: types.StringValue(m.ScopeType),
		ScopeID:   types.StringValue(m.ScopeID),
		StartsAt:  keepInstant(prior.StartsAt, m.StartsAt),
		EndsAt:    keepInstant(prior.EndsAt, m.EndsAt),
		Reason:    types.StringPointerValue(m.Reason),
		ServiceID: types.StringPointerValue(m.ServiceID),
		Active:    types.BoolValue(m.Active),
	}
}

// keepInstant returns prior when it is the same instant as the server value, which comes back in UTC.
func keepInstant(prior types.String, server string) types.String {
	if sameInstant(prior.ValueString(), server) {
		return prior
	}
	return types.StringValue(server)
}

// replaceUnlessSameInstant forces a new object when a time changes, but not when it is only spelled differently.
func replaceUnlessSameInstant() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !sameInstant(req.StateValue.ValueString(), req.PlanValue.ValueString())
		},
		"Changing the time replaces the object.", "Changing the time replaces the object.",
	)
}

// windowOver reports whether an RFC 3339 end time has passed.
func windowOver(endsAt types.String) bool {
	t, err := time.Parse(time.RFC3339, endsAt.ValueString())
	return err == nil && !t.After(time.Now())
}
