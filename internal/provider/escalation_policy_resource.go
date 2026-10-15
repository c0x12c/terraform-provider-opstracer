package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &escalationPolicyResource{}
	_ resource.ResourceWithConfigure   = &escalationPolicyResource{}
	_ resource.ResourceWithImportState = &escalationPolicyResource{}
	_ resource.ResourceWithModifyPlan  = &escalationPolicyResource{}
)

type escalationPolicyResource struct {
	client *client.Client
}

type escalationPolicyModel struct {
	ID                types.String  `tfsdk:"id"`
	Name              types.String  `tfsdk:"name"`
	Description       types.String  `tfsdk:"description"`
	Levels            []policyLevel `tfsdk:"levels"`
	RepeatCount       types.Int64   `tfsdk:"repeat_count"`
	AckTimeoutSeconds types.Int64   `tfsdk:"ack_timeout_seconds"`
	RepageAfterAck    types.Bool    `tfsdk:"repage_after_ack"`
	IsDefault         types.Bool    `tfsdk:"is_default"`
}

type policyLevel struct {
	TimeoutSeconds types.Int64    `tfsdk:"timeout_seconds"`
	Targets        []policyTarget `tfsdk:"targets"`
}

type policyTarget struct {
	Type       types.String `tfsdk:"type"`
	UserID     types.String `tfsdk:"user_id"`
	ScheduleID types.String `tfsdk:"schedule_id"`
}

func NewEscalationPolicyResource() resource.Resource {
	return &escalationPolicyResource{}
}

func (r *escalationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_policy"
}

func (r *escalationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An OpsTracer escalation policy: ordered levels of responders paged until someone acknowledges.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Escalation policy ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Policy name.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Free-text description.",
				Optional:    true,
			},
			"levels": schema.ListNestedAttribute{
				Required:    true,
				Description: "Escalation levels, paged in order.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timeout_seconds": schema.Int64Attribute{
							Optional:      true,
							Computed:      true,
							Description:   "Seconds to wait for an ack before moving to the next level. Server default is 300.",
							PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
						},
						"targets": schema.ListNestedAttribute{
							Description: "Who to page at this level. All targets are paged together.",
							Required:    true,
							Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{
										Required:    true,
										Description: "`user` or `schedule`.",
										Validators:  []validator.String{stringvalidator.OneOf("user", "schedule")},
									},
									"user_id": schema.StringAttribute{
										Description: "User to page. Required when `type` is `user`.",
										Optional:    true,
									},
									"schedule_id": schema.StringAttribute{
										Description: "Schedule whose on-call user is paged. Required when `type` is `schedule`.",
										Optional:    true,
									},
								},
							},
						},
					},
				},
			},
			// Server fills defaults when these are omitted.
			"repeat_count": schema.Int64Attribute{
				Description:   "Times to repeat the whole policy when nobody acknowledges. Server default when unset.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"ack_timeout_seconds": schema.Int64Attribute{
				Description:   "Seconds an acknowledgement lasts before the alert re-pages. Server default when unset.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"repage_after_ack": schema.BoolAttribute{
				Description:   "Whether to page again once the acknowledgement times out. Server default when unset.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"is_default": schema.BoolAttribute{
				Description: "Whether this is the org default policy, used when no routing rule matches. The org always has " +
					"exactly one default, so `true` takes it over from the current default. `false` is accepted only while " +
					"another policy is the default: to move the default, set `true` on the new policy and leave this unset here.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ModifyPlan rejects is_default = false on the current default at plan time, before anything is changed.
func (r *escalationPolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var planned, current types.Bool
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("is_default"), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("is_default"), &current)...)
	if planned.Equal(types.BoolValue(false)) && current.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("is_default"), "Cannot unset the default escalation policy", errUnsetDefault)
	}
}

const errUnsetDefault = "This policy is the org default, and the org always has exactly one. Set is_default = true " +
	"on another policy instead, and leave is_default unset on this one."

func (r *escalationPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *escalationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.CreateEscalationPolicy(ctx, escalationPolicyInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating escalation policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationPolicyFromAPI(p))...)
	if p, err = r.applyDefault(ctx, p, plan.IsDefault); err != nil {
		resp.Diagnostics.AddError("Error setting default escalation policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationPolicyFromAPI(p))...)
}

func (r *escalationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.GetEscalationPolicy(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading escalation policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationPolicyFromAPI(p))...)
}

func (r *escalationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.UpdateEscalationPolicy(ctx, escalationPolicyInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating escalation policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationPolicyFromAPI(p))...)
	if p, err = r.applyDefault(ctx, p, plan.IsDefault); err != nil {
		resp.Diagnostics.AddError("Error setting default escalation policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, escalationPolicyFromAPI(p))...)
}

func (r *escalationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteEscalationPolicy(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting escalation policy", err.Error())
	}
}

func (r *escalationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyDefault makes p the default when asked. There is no "unset" call: the default only moves when
// another policy takes it, so false on the current default is an error rather than a silent no-op.
func (r *escalationPolicyResource) applyDefault(ctx context.Context, p *client.EscalationPolicy, want types.Bool) (*client.EscalationPolicy, error) {
	switch {
	case want.IsUnknown() || want.IsNull() || want.ValueBool() == p.IsDefault:
		return p, nil
	case want.ValueBool():
		return r.client.SetDefaultEscalationPolicy(ctx, p.ID)
	default:
		return p, errors.New(errUnsetDefault)
	}
}

func escalationPolicyInput(m escalationPolicyModel) client.EscalationPolicyInput {
	in := client.EscalationPolicyInput{
		ID:             m.ID.ValueString(),
		Name:           m.Name.ValueString(),
		Description:    m.Description.ValueStringPointer(),
		AckTimeoutS:    knownInt64(m.AckTimeoutSeconds),
		RepageAfterAck: knownBool(m.RepageAfterAck),
	}
	if v := knownInt64(m.RepeatCount); v != nil {
		rc := int32(*v)
		in.RepeatCount = &rc
	}
	for _, l := range m.Levels {
		level := client.PolicyLevel{TimeoutS: knownInt64(l.TimeoutSeconds)}
		for _, t := range l.Targets {
			level.Targets = append(level.Targets, client.PolicyTarget{
				Type:       t.Type.ValueString(),
				UserID:     t.UserID.ValueStringPointer(),
				ScheduleID: t.ScheduleID.ValueStringPointer(),
			})
		}
		in.Levels = append(in.Levels, level)
	}
	return in
}

// Unknown (Optional+Computed left unset) must go out as null so the server applies its default;
// ValueXxxPointer would send the zero value instead.
func knownInt64(v types.Int64) *int64 {
	if v.IsUnknown() {
		return nil
	}
	return v.ValueInt64Pointer()
}

func knownBool(v types.Bool) *bool {
	if v.IsUnknown() {
		return nil
	}
	return v.ValueBoolPointer()
}

func escalationPolicyFromAPI(p *client.EscalationPolicy) escalationPolicyModel {
	m := escalationPolicyModel{
		ID:                types.StringValue(p.ID),
		Name:              types.StringValue(p.Name),
		Description:       types.StringPointerValue(p.Description),
		RepeatCount:       types.Int64Value(int64(p.RepeatCount)),
		AckTimeoutSeconds: types.Int64Value(p.AckTimeoutS),
		RepageAfterAck:    types.BoolValue(p.RepageAfterAck),
		IsDefault:         types.BoolValue(p.IsDefault),
	}
	for _, l := range p.Levels {
		level := policyLevel{TimeoutSeconds: types.Int64PointerValue(l.TimeoutS)}
		for _, t := range l.Targets {
			level.Targets = append(level.Targets, policyTarget{
				Type:       types.StringValue(t.Type),
				UserID:     types.StringPointerValue(t.UserID),
				ScheduleID: types.StringPointerValue(t.ScheduleID),
			})
		}
		m.Levels = append(m.Levels, level)
	}
	return m
}
