package provider

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &scheduleOverrideResource{}
	_ resource.ResourceWithConfigure   = &scheduleOverrideResource{}
	_ resource.ResourceWithImportState = &scheduleOverrideResource{}
)

// Import has no start time to search around, so it scans this far back and ahead.
const (
	overrideImportLookBack  = 365 * 24 * time.Hour
	overrideImportLookAhead = 2 * 365 * 24 * time.Hour
)

type scheduleOverrideResource struct {
	client *client.Client
}

type scheduleOverrideModel struct {
	ID             types.String `tfsdk:"id"`
	ScheduleID     types.String `tfsdk:"schedule_id"`
	UserID         types.String `tfsdk:"user_id"`
	StartsAt       types.String `tfsdk:"starts_at"`
	EndsAt         types.String `tfsdk:"ends_at"`
	ReplacesUserID types.String `tfsdk:"replaces_user_id"`
}

func NewScheduleOverrideResource() resource.Resource {
	return &scheduleOverrideResource{}
}

func (r *scheduleOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_override"
}

func (r *scheduleOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	replaceTime := []planmodifier.String{sameInstantAsState{}, stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Puts someone on call for a schedule for a fixed window, on top of the rotation. " +
			"Overrides cannot be edited, so changing any argument replaces it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Override ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"schedule_id": schema.StringAttribute{
				Description:   "Schedule to override.",
				Required:      true,
				PlanModifiers: replace,
			},
			"user_id": schema.StringAttribute{
				Description:   "User on call during the window. Must be an org member.",
				Required:      true,
				PlanModifiers: replace,
			},
			"starts_at": schema.StringAttribute{
				Description:   "RFC 3339 start of the window, e.g. `2026-11-20T09:00:00+07:00`.",
				Required:      true,
				Validators:    []validator.String{rfc3339Validator{}},
				PlanModifiers: replaceTime,
			},
			"ends_at": schema.StringAttribute{
				Description:   "RFC 3339 end of the window. Must be after `starts_at`.",
				Required:      true,
				Validators:    []validator.String{rfc3339Validator{}},
				PlanModifiers: replaceTime,
			},
			"replaces_user_id": schema.StringAttribute{
				Description:   "User this override stands in for. Leave unset to cover whoever is on call.",
				Optional:      true,
				PlanModifiers: replace,
			},
		},
	}
}

func (r *scheduleOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *scheduleOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduleOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	o, err := r.client.CreateScheduleOverride(ctx, client.ScheduleOverride{
		ScheduleID:     plan.ScheduleID.ValueString(),
		UserID:         plan.UserID.ValueString(),
		StartsAt:       plan.StartsAt.ValueString(),
		EndsAt:         plan.EndsAt.ValueString(),
		ReplacesUserID: plan.ReplacesUserID.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating schedule override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, overrideFromAPI(o, plan))...)
}

// There is no get-by-id endpoint. The list takes a time window (at most 90 days) and returns the
// overrides overlapping it, so a one-second window at starts_at always contains this override.
func (r *scheduleOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduleOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	from, to := time.Now().Add(-overrideImportLookBack), time.Now().Add(overrideImportLookAhead)
	if start, err := time.Parse(time.RFC3339, state.StartsAt.ValueString()); err == nil {
		from, to = start, start.Add(time.Second)
	}
	for ; from.Before(to); from = from.Add(client.MaxOverrideListWindow) {
		end := from.Add(client.MaxOverrideListWindow)
		if end.After(to) {
			end = to
		}
		overrides, err := r.client.ListScheduleOverrides(ctx, state.ScheduleID.ValueString(), from, end)
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Error listing schedule overrides", err.Error())
			return
		}
		for _, o := range overrides {
			if o.ID == state.ID.ValueString() {
				resp.Diagnostics.Append(resp.State.Set(ctx, overrideFromAPI(&o, state))...)
				return
			}
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *scheduleOverrideResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Schedule overrides cannot be updated", "Every argument forces replacement; this is a provider bug.")
}

func (r *scheduleOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduleOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteScheduleOverride(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting schedule override", err.Error())
	}
}

// ImportState takes `<schedule_id>/<override_id>`.
func (r *scheduleOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scheduleID, overrideID, ok := strings.Cut(req.ID, "/")
	if !ok || scheduleID == "" || overrideID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected <schedule_id>/<override_id>, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schedule_id"), scheduleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), overrideID)...)
}

// overrideFromAPI keeps prior's time spelling when it is the same instant the server returned in UTC.
func overrideFromAPI(o *client.ScheduleOverride, prior scheduleOverrideModel) scheduleOverrideModel {
	m := scheduleOverrideModel{
		ID:             types.StringValue(o.ID),
		ScheduleID:     types.StringValue(o.ScheduleID),
		UserID:         types.StringValue(o.UserID),
		StartsAt:       types.StringValue(o.StartsAt),
		EndsAt:         types.StringValue(o.EndsAt),
		ReplacesUserID: types.StringPointerValue(o.ReplacesUserID),
	}
	if sameInstant(prior.StartsAt.ValueString(), o.StartsAt) {
		m.StartsAt = prior.StartsAt
	}
	if sameInstant(prior.EndsAt.ValueString(), o.EndsAt) {
		m.EndsAt = prior.EndsAt
	}
	return m
}

// sameInstantAsState keeps the state spelling of a timestamp (UTC after import) when the config names the same instant.
type sameInstantAsState struct{}

func (sameInstantAsState) Description(context.Context) string {
	return "A different spelling of the same instant is not a change."
}

func (m sameInstantAsState) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (sameInstantAsState) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.StateValue.IsNull() && !req.PlanValue.IsUnknown() && sameInstant(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}
