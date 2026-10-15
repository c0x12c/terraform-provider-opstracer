package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &logAlertMonitorResource{}
	_ resource.ResourceWithConfigure   = &logAlertMonitorResource{}
	_ resource.ResourceWithImportState = &logAlertMonitorResource{}
)

type logAlertMonitorResource struct {
	client *client.Client
}

type logAlertMonitorModel struct {
	ID            types.String      `tfsdk:"id"`
	SourceType    types.String      `tfsdk:"source_type"`
	Scope         map[string]string `tfsdk:"scope"`
	IntegrationID types.String      `tfsdk:"integration_id"`
	ConnectionID  types.String      `tfsdk:"connection_id"`
	Enabled       types.Bool        `tfsdk:"enabled"`
	ScopeName     types.String      `tfsdk:"scope_name"`
}

func NewLogAlertMonitorResource() resource.Resource {
	return &logAlertMonitorResource{}
}

func (r *logAlertMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_alert_monitor"
}

func (r *logAlertMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Polls the logs of one source (e.g. a Railway service) so that `opstracer_log_alert_rule` " +
			"rules can raise alerts from them. Log alerts must be enabled for the org. Only `enabled` can change " +
			"in place; other changes replace the monitor, and deleting it deletes its rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Monitor ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"source_type": schema.StringAttribute{
				Description:   "Log source type, e.g. `railway`.",
				Required:      true,
				PlanModifiers: replace,
			},
			"scope": schema.MapAttribute{
				Description:   "Which logs of the source to poll, e.g. Railway `project_id`, `environment_id` and `service_id`.",
				Required:      true,
				ElementType:   types.StringType,
				Validators:    []validator.Map{mapvalidator.SizeAtLeast(1)},
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"integration_id": schema.StringAttribute{
				Description:   "Integration that receives the alerts the rules raise.",
				Required:      true,
				PlanModifiers: replace,
			},
			"connection_id": schema.StringAttribute{
				Description:   "Observability connection used to read the logs. Defaults to the org default connection for the source.",
				Optional:      true,
				PlanModifiers: replace,
			},
			"enabled": schema.BoolAttribute{
				Description: "Set to false to stop polling without deleting the monitor.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"scope_name": schema.StringAttribute{
				Description:   "Display name of the scope.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *logAlertMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *logAlertMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logAlertMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.client.CreateLogMonitor(ctx, client.LogMonitorInput{
		SourceType:    plan.SourceType.ValueString(),
		Scope:         plan.Scope,
		IntegrationID: plan.IntegrationID.ValueString(),
		ConnectionID:  plan.ConnectionID.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating log alert monitor", err.Error())
		return
	}
	// Save the ID first so a failed enable/disable does not leak the monitor.
	resp.Diagnostics.Append(resp.State.Set(ctx, logMonitorFromAPI(m, plan))...)
	if m.Enabled != plan.Enabled.ValueBool() {
		if m, err = r.client.SetLogMonitorEnabled(ctx, m.ID, plan.Enabled.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Error enabling or disabling log alert monitor", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, logMonitorFromAPI(m, plan))...)
	}
}

// There is no get-by-id endpoint, so Read scans the list.
func (r *logAlertMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logAlertMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	monitors, err := r.client.ListLogMonitors(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading log alert monitors", err.Error())
		return
	}
	for _, m := range monitors {
		if m.ID == state.ID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, logMonitorFromAPI(&m, state))...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *logAlertMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logAlertMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.client.SetLogMonitorEnabled(ctx, plan.ID.ValueString(), plan.Enabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling or disabling log alert monitor", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logMonitorFromAPI(m, plan))...)
}

func (r *logAlertMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logAlertMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteLogMonitor(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log alert monitor", err.Error())
	}
}

func (r *logAlertMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func logMonitorFromAPI(m *client.LogMonitor, prior logAlertMonitorModel) logAlertMonitorModel {
	out := logAlertMonitorModel{
		ID:            types.StringValue(m.ID),
		SourceType:    types.StringValue(m.SourceType),
		Scope:         prior.Scope,
		IntegrationID: types.StringValue(m.IntegrationID),
		ConnectionID:  prior.ConnectionID,
		Enabled:       types.BoolValue(m.Enabled),
		ScopeName:     types.StringPointerValue(m.ScopeName),
	}
	// The spec documents the response scope as a list, so only use it when it is the flat map that was sent.
	var scope map[string]string
	if json.Unmarshal(m.Scope, &scope) == nil && len(scope) > 0 {
		out.Scope = scope
	}
	return out
}
