package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

const routingRulesID = "routing_rules"

var (
	_ resource.Resource                = &routingRulesResource{}
	_ resource.ResourceWithConfigure   = &routingRulesResource{}
	_ resource.ResourceWithImportState = &routingRulesResource{}
)

type routingRulesResource struct {
	client *client.Client
}

type routingRulesModel struct {
	ID    types.String       `tfsdk:"id"`
	Rules []routingRuleModel `tfsdk:"rules"`
}

type routingRuleModel struct {
	ID                 types.String      `tfsdk:"id"`
	EscalationPolicyID types.String      `tfsdk:"escalation_policy_id"`
	Enabled            types.Bool        `tfsdk:"enabled"`
	Severities         []string          `tfsdk:"severities"`
	SourceType         types.String      `tfsdk:"source_type"`
	IntegrationID      types.String      `tfsdk:"integration_id"`
	Tags               map[string]string `tfsdk:"tags"`
	RoutingHints       map[string]string `tfsdk:"routing_hints"`
}

var sourceTypes = []string{
	"alertmanager", "datadog", "generic", "grafana", "newrelic",
	"pagerduty", "railway", "sentry", "supabase", "vercel",
}

func NewRoutingRulesResource() resource.Resource {
	return &routingRulesResource{}
}

func (r *routingRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routing_rules"
}

func (r *routingRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All alert routing rules of the org, in evaluation order: the first matching rule picks the " +
			"escalation policy, otherwise the service policy applies. Authoritative: rules not listed here are deleted. " +
			"Use one instance per org.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Always `routing_rules`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"rules": schema.ListNestedAttribute{
				Description: "Rules in evaluation order.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description:   "Rule ID.",
							Computed:      true,
							PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"escalation_policy_id": schema.StringAttribute{
							Description: "Escalation policy to page when the rule matches.",
							Required:    true,
						},
						"enabled": schema.BoolAttribute{
							Description: "Set to false to keep the rule without evaluating it.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
						},
						"severities": schema.SetAttribute{
							Optional:    true,
							ElementType: types.StringType,
							Description: "Match any of P1 ... P5.",
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
								setvalidator.ValueStringsAre(stringvalidator.OneOf("P1", "P2", "P3", "P4", "P5")),
							},
						},
						"source_type": schema.StringAttribute{
							Description: "Match alerts from this integration type: `alertmanager`, `datadog`, `generic`, `grafana`, `newrelic`, `pagerduty`, `railway`, `sentry`, `supabase`, `vercel`.",
							Optional:    true,
							Validators:  []validator.String{stringvalidator.OneOf(sourceTypes...)},
						},
						"integration_id": schema.StringAttribute{
							Description: "Match alerts from this integration only.",
							Optional:    true,
						},
						"tags": schema.MapAttribute{
							Optional:    true,
							ElementType: types.StringType,
							Description: "Alert tags that must all match.",
							Validators:  []validator.Map{mapvalidator.SizeAtLeast(1)},
						},
						"routing_hints": schema.MapAttribute{
							Description: "Routing hints on the alert that must all match.",
							Optional:    true,
							ElementType: types.StringType,
							Validators:  []validator.Map{mapvalidator.SizeAtLeast(1)},
						},
					},
				},
			},
		},
	}
}

func (r *routingRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

// Create adopts any rules already in the org, the same way Update reuses rules from state.
func (r *routingRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan routingRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, err := r.client.ListRoutingRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing routing rules", err.Error())
		return
	}
	ids := make([]string, len(existing))
	for i, rule := range existing {
		ids[i] = rule.ID
	}
	resp.Diagnostics.Append(r.apply(ctx, ids, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *routingRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state routingRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.read(ctx, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *routingRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state routingRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids := make([]string, len(state.Rules))
	for i, rule := range state.Rules {
		ids[i] = rule.ID.ValueString()
	}
	resp.Diagnostics.Append(r.apply(ctx, ids, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *routingRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state routingRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, rule := range state.Rules {
		if err := r.client.DeleteRoutingRule(ctx, rule.ID.ValueString()); err != nil && !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error deleting routing rule", err.Error())
			return
		}
	}
}

func (r *routingRulesResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), routingRulesID)...)
}

// apply reuses existing rule IDs by position: rule i updates existingIDs[i], extra rules are created
// and leftover existing rules are deleted. Every update keeps the ID, so positions stay aligned.
func (r *routingRulesResource) apply(ctx context.Context, existingIDs []string, plan *routingRulesModel) diag.Diagnostics {
	var diags diag.Diagnostics
	ids := make([]string, len(plan.Rules))
	for i, rule := range plan.Rules {
		in := routingRuleInput(rule)
		var (
			out *client.RoutingRule
			err error
		)
		if i < len(existingIDs) {
			in.ID = existingIDs[i]
			out, err = r.client.UpdateRoutingRule(ctx, in)
		} else {
			out, err = r.client.CreateRoutingRule(ctx, in)
		}
		if err != nil {
			diags.AddError("Error saving routing rule", err.Error())
			return diags
		}
		ids[i] = out.ID
	}
	for _, id := range existingIDs[min(len(plan.Rules), len(existingIDs)):] {
		if err := r.client.DeleteRoutingRule(ctx, id); err != nil && !client.IsNotFound(err) {
			diags.AddError("Error deleting routing rule", err.Error())
			return diags
		}
	}
	if len(ids) > 1 {
		if err := r.client.ReorderRoutingRules(ctx, ids); err != nil {
			diags.AddError("Error ordering routing rules", err.Error())
			return diags
		}
	}
	plan.ID = types.StringValue(routingRulesID)
	diags.Append(r.read(ctx, plan)...)
	return diags
}

func (r *routingRulesResource) read(ctx context.Context, m *routingRulesModel) diag.Diagnostics {
	var diags diag.Diagnostics
	rules, err := r.client.ListRoutingRules(ctx)
	if err != nil {
		diags.AddError("Error listing routing rules", err.Error())
		return diags
	}
	m.ID = types.StringValue(routingRulesID)
	m.Rules = []routingRuleModel{}
	for _, rule := range rules {
		m.Rules = append(m.Rules, routingRuleModel{
			ID:                 types.StringValue(rule.ID),
			EscalationPolicyID: types.StringValue(rule.Policy.ID),
			Enabled:            types.BoolValue(rule.Enabled),
			Severities:         rule.Matcher.Severity,
			SourceType:         types.StringPointerValue(rule.Matcher.SourceType),
			IntegrationID:      types.StringPointerValue(rule.Matcher.IntegrationID),
			Tags:               rule.Matcher.Tags,
			RoutingHints:       rule.Matcher.RoutingHints,
		})
	}
	return diags
}

func routingRuleInput(m routingRuleModel) client.RoutingRuleInput {
	return client.RoutingRuleInput{
		PolicyID: m.EscalationPolicyID.ValueString(),
		Enabled:  m.Enabled.ValueBool(),
		Matcher: client.Matcher{
			Severity:      m.Severities,
			SourceType:    m.SourceType.ValueStringPointer(),
			IntegrationID: m.IntegrationID.ValueStringPointer(),
			Tags:          m.Tags,
			RoutingHints:  m.RoutingHints,
		},
	}
}
