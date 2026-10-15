package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &suppressionResource{}
	_ resource.ResourceWithConfigure   = &suppressionResource{}
	_ resource.ResourceWithImportState = &suppressionResource{}
)

type suppressionResource struct {
	client *client.Client
}

type suppressionModel struct {
	ID            types.String      `tfsdk:"id"`
	ServiceID     types.String      `tfsdk:"service_id"`
	Severities    []string          `tfsdk:"severities"`
	SourceType    types.String      `tfsdk:"source_type"`
	IntegrationID types.String      `tfsdk:"integration_id"`
	Tags          map[string]string `tfsdk:"tags"`
	RoutingHints  map[string]string `tfsdk:"routing_hints"`
	StartsAt      types.String      `tfsdk:"starts_at"`
	EndsAt        types.String      `tfsdk:"ends_at"`
	Reason        types.String      `tfsdk:"reason"`
	Active        types.Bool        `tfsdk:"active"`
	MatchCount    types.Int64       `tfsdk:"match_count"`
}

func NewSuppressionResource() resource.Resource {
	return &suppressionResource{}
}

func (r *suppressionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_suppression"
}

func (r *suppressionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Silences only the alerts of one service that match all the given conditions, for a time " +
			"window, and counts how many it caught. With no condition it matches every alert of the service. To " +
			"silence a whole service or integration, use `opstracer_mute`. A suppression cannot be edited, so any " +
			"change replaces it, except writing the same time in another UTC offset. Once the window is over it stays in state as inactive; one ended early outside " +
			"Terraform is created again. Destroy ends a running suppression and cancels a scheduled one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Suppression ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Description:   "Service whose alerts are suppressed.",
				Required:      true,
				PlanModifiers: replace,
			},
			"severities": schema.SetAttribute{
				Description: "Match any of P1 ... P5.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf("P1", "P2", "P3", "P4", "P5")),
				},
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"source_type": schema.StringAttribute{
				Description:   "Match alerts from this integration type: `alertmanager`, `datadog`, `generic`, `grafana`, `newrelic`, `pagerduty`, `railway`, `sentry`, `supabase`, `vercel`.",
				Optional:      true,
				Validators:    []validator.String{stringvalidator.OneOf(sourceTypes...)},
				PlanModifiers: replace,
			},
			"integration_id": schema.StringAttribute{
				Description:   "Match alerts from this integration only.",
				Optional:      true,
				PlanModifiers: replace,
			},
			"tags": schema.MapAttribute{
				Description:   "Alert tags that must all match.",
				Optional:      true,
				ElementType:   types.StringType,
				Validators:    []validator.Map{mapvalidator.SizeAtLeast(1)},
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"routing_hints": schema.MapAttribute{
				Description:   "Routing hints on the alert that must all match.",
				Optional:      true,
				ElementType:   types.StringType,
				Validators:    []validator.Map{mapvalidator.SizeAtLeast(1)},
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
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
				Description:   "Why the alerts are suppressed.",
				Optional:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: replace,
			},
			"active": schema.BoolAttribute{
				Description:   "Whether the suppression is in effect right now. False before the start and after the end.",
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"match_count": schema.Int64Attribute{
				Description:   "Number of alerts suppressed so far.",
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *suppressionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *suppressionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan suppressionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.CreateSuppression(ctx, client.SuppressionInput{
		ServiceID: plan.ServiceID.ValueString(),
		Matcher: client.SuppressionMatcher{
			Severity:      plan.Severities,
			SourceType:    plan.SourceType.ValueStringPointer(),
			IntegrationID: plan.IntegrationID.ValueStringPointer(),
			Tags:          plan.Tags,
			RoutingHints:  plan.RoutingHints,
		},
		StartsAt: knownString(plan.StartsAt),
		EndsAt:   plan.EndsAt.ValueString(),
		Reason:   plan.Reason.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating suppression", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, suppressionFromAPI(s, plan))...)
}

// There is no get-by-id endpoint, so Read scans the service's list, which only has scheduled and running ones.
func (r *suppressionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state suppressionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := r.client.ListSuppressions(ctx, state.ServiceID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error reading suppressions", err.Error())
		return
	}
	for _, s := range list {
		if s.ID == state.ID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, suppressionFromAPI(&s, state))...)
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
func (r *suppressionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state suppressionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.StartsAt, state.EndsAt = plan.StartsAt, plan.EndsAt
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *suppressionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state suppressionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.EndSuppression(ctx, state.ID.ValueString())
	if client.ErrorCode(err) == "suppression_not_started" {
		err = r.client.CancelSuppression(ctx, state.ID.ValueString())
	}
	if err != nil && !client.IsNotFound(err) && client.ErrorCode(err) != "suppression_already_ended" {
		resp.Diagnostics.AddError("Error ending suppression", err.Error())
	}
}

// The list is per service, so the import ID is `<service_id>/<suppression_id>`.
func (r *suppressionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	serviceID, id, ok := strings.Cut(req.ID, "/")
	if !ok || serviceID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected <service_id>/<suppression_id>, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), serviceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func suppressionFromAPI(s *client.Suppression, prior suppressionModel) suppressionModel {
	return suppressionModel{
		ID:            types.StringValue(s.ID),
		ServiceID:     types.StringValue(s.ServiceID),
		Severities:    s.Matcher.Severity,
		SourceType:    types.StringPointerValue(s.Matcher.SourceType),
		IntegrationID: types.StringPointerValue(s.Matcher.IntegrationID),
		Tags:          s.Matcher.Tags,
		RoutingHints:  s.Matcher.RoutingHints,
		StartsAt:      keepInstant(prior.StartsAt, s.StartsAt),
		EndsAt:        keepInstant(prior.EndsAt, s.EndsAt),
		Reason:        types.StringPointerValue(s.Reason),
		Active:        types.BoolValue(s.Active),
		MatchCount:    types.Int64Value(s.MatchCount),
	}
}
