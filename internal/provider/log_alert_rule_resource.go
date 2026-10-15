package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &logAlertRuleResource{}
	_ resource.ResourceWithConfigure   = &logAlertRuleResource{}
	_ resource.ResourceWithImportState = &logAlertRuleResource{}
)

type logAlertRuleResource struct {
	client *client.Client
}

type logAlertRuleModel struct {
	ID            types.String `tfsdk:"id"`
	MonitorID     types.String `tfsdk:"monitor_id"`
	Name          types.String `tfsdk:"name"`
	ServiceID     types.String `tfsdk:"service_id"`
	ServiceName   types.String `tfsdk:"service_name"`
	MatchLevel    types.String `tfsdk:"match_level"`
	MatchText     []string     `tfsdk:"match_text"`
	Threshold     types.Int64  `tfsdk:"threshold"`
	WindowMinutes types.Int64  `tfsdk:"window_minutes"`
	Severity      types.String `tfsdk:"severity"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	State         types.String `tfsdk:"state"`
}

func NewLogAlertRuleResource() resource.Resource {
	return &logAlertRuleResource{}
}

func (r *logAlertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_alert_rule"
}

func (r *logAlertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A rule on an `opstracer_log_alert_monitor`: raises an alert when the number of matching " +
			"log lines within `window_minutes` crosses `threshold`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Rule ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"monitor_id": schema.StringAttribute{
				Description:   "Monitor whose logs the rule evaluates.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Description: "Rule name, used as the alert title.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"service_id": schema.StringAttribute{
				Description: "Only count log lines of this service of the source.",
				Optional:    true,
			},
			"service_name": schema.StringAttribute{
				Description: "Display name of `service_id`.",
				Optional:    true,
			},
			"match_level": schema.StringAttribute{
				Description: "Only count log lines of this level: `error` or `warn`.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf("error", "warn")},
			},
			"match_text": schema.ListAttribute{
				Description: "Only count log lines that contain any of these strings.",
				Optional:    true,
				ElementType: types.StringType,
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
			},
			"threshold": schema.Int64Attribute{
				Description: "Number of matching log lines in the window that raises the alert.",
				Required:    true,
				Validators:  []validator.Int64{int64validator.AtLeast(0)},
			},
			"window_minutes": schema.Int64Attribute{
				Description: "Length of the counting window, 1 to 60 minutes.",
				Required:    true,
				Validators:  []validator.Int64{int64validator.Between(1, 60)},
			},
			"severity": schema.StringAttribute{
				Description: "Severity of the raised alert, P1 ... P5.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf("P1", "P2", "P3", "P4", "P5")},
			},
			"enabled": schema.BoolAttribute{
				Description: "Set to false to keep the rule without evaluating it.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"state": schema.StringAttribute{
				Description: "Current evaluation state reported by the server.",
				Computed:    true,
			},
		},
	}
}

func (r *logAlertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *logAlertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := logAlertRuleInput(plan)
	in.MonitorID = plan.MonitorID.ValueString()
	rule, err := r.client.CreateLogAlertRule(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating log alert rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromAPI(rule))...)
}

// Rules come nested in the monitor list; there is no endpoint for a single rule.
func (r *logAlertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logAlertRuleModel
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
		for _, rule := range m.Rules {
			if rule.ID == state.ID.ValueString() {
				if rule.MonitorID == "" {
					rule.MonitorID = m.ID
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromAPI(&rule))...)
				return
			}
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *logAlertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := logAlertRuleInput(plan)
	in.ID = plan.ID.ValueString()
	rule, err := r.client.UpdateLogAlertRule(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating log alert rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, logAlertRuleFromAPI(rule))...)
}

func (r *logAlertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state logAlertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteLogAlertRule(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log alert rule", err.Error())
	}
}

func (r *logAlertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func logAlertRuleInput(m logAlertRuleModel) client.LogAlertRuleInput {
	in := client.LogAlertRuleInput{
		Name:          m.Name.ValueString(),
		ServiceID:     m.ServiceID.ValueStringPointer(),
		ServiceName:   m.ServiceName.ValueStringPointer(),
		MatchLevel:    m.MatchLevel.ValueStringPointer(),
		MatchText:     m.MatchText,
		Threshold:     m.Threshold.ValueInt64(),
		WindowMinutes: m.WindowMinutes.ValueInt64(),
		Severity:      m.Severity.ValueString(),
		Enabled:       m.Enabled.ValueBool(),
	}
	if in.MatchText == nil {
		in.MatchText = []string{}
	}
	return in
}

// An empty match_text is omitted by the server and maps back to null.
func logAlertRuleFromAPI(r *client.LogAlertRule) logAlertRuleModel {
	m := logAlertRuleModel{
		ID:            types.StringValue(r.ID),
		MonitorID:     types.StringValue(r.MonitorID),
		Name:          types.StringValue(r.Name),
		ServiceID:     types.StringPointerValue(r.ServiceID),
		ServiceName:   types.StringPointerValue(r.ServiceName),
		MatchLevel:    types.StringPointerValue(r.MatchLevel),
		Threshold:     types.Int64Value(r.Threshold),
		WindowMinutes: types.Int64Value(r.WindowMinutes),
		Severity:      types.StringValue(r.Severity),
		Enabled:       types.BoolValue(r.Enabled),
		State:         types.StringValue(r.State),
	}
	if len(r.MatchText) > 0 {
		m.MatchText = r.MatchText
	}
	return m
}
