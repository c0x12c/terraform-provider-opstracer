package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &integrationResource{}
	_ resource.ResourceWithConfigure   = &integrationResource{}
	_ resource.ResourceWithImportState = &integrationResource{}
	_ resource.ResourceWithModifyPlan  = &integrationResource{}
)

var severities = []string{"P1", "P2", "P3", "P4", "P5"}

type integrationResource struct {
	client *client.Client
}

type integrationModel struct {
	ID               types.String `tfsdk:"id"`
	ServiceID        types.String `tfsdk:"service_id"`
	Name             types.String `tfsdk:"name"`
	Type             types.String `tfsdk:"type"`
	Status           types.String `tfsdk:"status"`
	DefaultSeverity  types.String `tfsdk:"default_severity"`
	SeverityMapping  types.Map    `tfsdk:"severity_mapping"`
	RateSoftPerMin   types.Int64  `tfsdk:"rate_soft_per_min"`
	RateHardPerMin   types.Int64  `tfsdk:"rate_hard_per_min"`
	RotateKeyTrigger types.String `tfsdk:"rotate_key_trigger"`
	IntegrationURL   types.String `tfsdk:"integration_url"`
	RoutingKey       types.String `tfsdk:"routing_key"`
	TokenHint        types.String `tfsdk:"token_hint"`
}

func NewIntegrationResource() resource.Resource {
	return &integrationResource{}
}

func (r *integrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *integrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An alert source integration: an ingest endpoint and routing key that a monitoring tool sends alerts to for one service.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Integration ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Description:   "ID of the service the alerts belong to. Changing it replaces the integration.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Description: "Integration name.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description:   "Alert source type, e.g. `generic`, `datadog`, `grafana`. Changing it replaces the integration.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Description:   "`active` or `disabled`. Defaults to `active`.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf("active", "disabled")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"default_severity": schema.StringAttribute{
				Description:   "Severity (`P1` to `P5`) for alerts that carry none. Server default is `P3`.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf(severities...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"severity_mapping": schema.MapAttribute{
				Description: "Maps the source's own severity values to OpsTracer severities (`P1` to `P5`).",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
					mapvalidator.ValueStringsAre(stringvalidator.OneOf(severities...)),
				},
			},
			"rate_soft_per_min": schema.Int64Attribute{
				Description:   "Alerts per minute above which ingestion is throttled. Server default is 600.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"rate_hard_per_min": schema.Int64Attribute{
				Description:   "Alerts per minute above which events are dropped. Must not be below `rate_soft_per_min`. Server default is 6000.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"rotate_key_trigger": schema.StringAttribute{
				Description: "Any value; changing it issues a new routing key and retires the old one immediately. " +
					"After an import the first value is adopted without rotating.",
				Optional: true,
			},
			"integration_url": schema.StringAttribute{
				Description:   "Ingest URL the alert source posts to.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"routing_key": schema.StringAttribute{
				Description:   "Routing key the alert source authenticates with. Only known when Terraform created or rotated it; null after import.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token_hint": schema.StringAttribute{
				Description:   "Last characters of the current routing key.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *integrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

// ModifyPlan marks the key attributes unknown when a rotation is planned.
func (r *integrationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	imported, diags := req.Private.GetKey(ctx, importedKey)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || !rotationPlanned(plan, state, imported) {
		return
	}
	plan.RoutingKey = types.StringUnknown()
	plan.TokenHint = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

func (r *integrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := integrationUpdate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, key, err := r.client.CreateIntegration(ctx, client.IntegrationInput{
		ServiceID:       plan.ServiceID.ValueString(),
		Name:            plan.Name.ValueString(),
		Type:            plan.Type.ValueString(),
		DefaultSeverity: knownString(plan.DefaultSeverity),
		RateSoftPerMin:  knownInt64(plan.RateSoftPerMin),
		RateHardPerMin:  knownInt64(plan.RateHardPerMin),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating integration", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	plan.RoutingKey = types.StringValue(key)
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromAPI(ctx, created, plan))...)

	// Status and severity mapping can only be set by an update.
	if in.Status == nil && in.SeverityMapping == nil {
		return
	}
	in.ID = created.ID
	updated, err := r.client.UpdateIntegration(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error configuring integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromAPI(ctx, updated, plan))...)
}

func (r *integrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	i, err := r.client.GetIntegration(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromAPI(ctx, i, state))...)
}

func (r *integrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, diags := integrationUpdate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	in.ID = state.ID.ValueString()
	if in.SeverityMapping == nil {
		in.SeverityMapping = map[string]string{}
	}
	i, err := r.client.UpdateIntegration(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating integration", err.Error())
		return
	}

	plan.RoutingKey = state.RoutingKey
	imported, diags := req.Private.GetKey(ctx, importedKey)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, nil)...)
	if rotationPlanned(plan, state, imported) {
		var key string
		if i, key, err = r.client.RotateIntegrationKey(ctx, in.ID); err != nil {
			resp.Diagnostics.AddError("Error rotating integration key", err.Error())
			return
		}
		plan.RoutingKey = types.StringValue(key)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationFromAPI(ctx, i, plan))...)
}

func (r *integrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIntegration(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting integration", err.Error())
	}
}

func (r *integrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, []byte("true"))...)
}

// The first trigger value set after an import is adopted without rotating the live key.
func rotationPlanned(plan, state integrationModel, imported []byte) bool {
	if state.RotateKeyTrigger.IsNull() && imported != nil {
		return false
	}
	return !plan.RotateKeyTrigger.Equal(state.RotateKeyTrigger)
}

func integrationUpdate(ctx context.Context, m integrationModel) (client.IntegrationUpdate, diag.Diagnostics) {
	in := client.IntegrationUpdate{
		Name:            m.Name.ValueStringPointer(),
		Status:          knownString(m.Status),
		DefaultSeverity: knownString(m.DefaultSeverity),
		RateSoftPerMin:  knownInt64(m.RateSoftPerMin),
		RateHardPerMin:  knownInt64(m.RateHardPerMin),
	}
	var diags diag.Diagnostics
	if !m.SeverityMapping.IsNull() {
		diags = m.SeverityMapping.ElementsAs(ctx, &in.SeverityMapping, false)
	}
	return in, diags
}

// prior supplies the values the API never returns: the routing key and the rotation trigger.
func integrationFromAPI(ctx context.Context, i *client.Integration, prior integrationModel) integrationModel {
	mapping := types.MapNull(types.StringType)
	if len(i.SeverityMapping) > 0 {
		mapping, _ = types.MapValueFrom(ctx, types.StringType, i.SeverityMapping)
	}
	return integrationModel{
		ID:               types.StringValue(i.ID),
		ServiceID:        types.StringValue(i.ServiceID),
		Name:             types.StringValue(i.Name),
		Type:             types.StringValue(i.Type),
		Status:           types.StringValue(i.Status),
		DefaultSeverity:  types.StringValue(i.DefaultSeverity),
		SeverityMapping:  mapping,
		RateSoftPerMin:   types.Int64Value(i.RateSoftPerMin),
		RateHardPerMin:   types.Int64Value(i.RateHardPerMin),
		RotateKeyTrigger: prior.RotateKeyTrigger,
		IntegrationURL:   types.StringValue(i.IntegrationURL),
		RoutingKey:       prior.RoutingKey,
		TokenHint:        types.StringValue(i.TokenHint),
	}
}
