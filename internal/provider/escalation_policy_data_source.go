package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource                     = &escalationPolicyDataSource{}
	_ datasource.DataSourceWithConfigure        = &escalationPolicyDataSource{}
	_ datasource.DataSourceWithConfigValidators = &escalationPolicyDataSource{}
)

type escalationPolicyDataSource struct {
	client *client.Client
}

type escalationPolicyLookupModel struct {
	Name        types.String `tfsdk:"name"`
	Default     types.Bool   `tfsdk:"default"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	IsDefault   types.Bool   `tfsdk:"is_default"`
}

func NewEscalationPolicyDataSource() datasource.DataSource {
	return &escalationPolicyDataSource{}
}

func (d *escalationPolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_policy"
}

func (d *escalationPolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an escalation policy by its exact name, or the org's default policy.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Policy name, matched exactly. Set this or `default`.",
			},
			"default": schema.BoolAttribute{
				Optional:    true,
				Description: "Set to `true` to fetch the org's default policy instead of matching by name.",
			},
			"id":          schema.StringAttribute{Computed: true, Description: "Escalation policy ID."},
			"description": schema.StringAttribute{Computed: true, Description: "Free-text description."},
			"is_default":  schema.BoolAttribute{Computed: true, Description: "Whether this is the org's default policy."},
		},
	}
}

func (d *escalationPolicyDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("name"), path.MatchRoot("default")),
	}
}

func (d *escalationPolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *escalationPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg escalationPolicyLookupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var p *client.EscalationPolicy
	if !cfg.Default.IsNull() {
		if !cfg.Default.ValueBool() {
			resp.Diagnostics.AddAttributeError(path.Root("default"), "Invalid default", "`default` can only be `true`; use `name` to match a policy by name.")
			return
		}
		var err error
		if p, err = d.client.GetDefaultEscalationPolicy(ctx); err != nil {
			resp.Diagnostics.AddError("Error reading default escalation policy", err.Error())
			return
		}
	} else {
		policies, err := d.client.ListEscalationPolicies(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing escalation policies", err.Error())
			return
		}
		found, err := findByName("escalation policy", cfg.Name.ValueString(), policies,
			func(p client.EscalationPolicy) string { return p.Name }, func(p client.EscalationPolicy) string { return p.ID })
		if err != nil {
			resp.Diagnostics.AddError("Escalation policy lookup failed", err.Error())
			return
		}
		p = &found
	}
	cfg.ID = types.StringValue(p.ID)
	cfg.Name = types.StringValue(p.Name)
	cfg.Description = types.StringPointerValue(p.Description)
	cfg.IsDefault = types.BoolValue(p.IsDefault)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
