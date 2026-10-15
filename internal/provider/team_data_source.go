package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource              = &teamDataSource{}
	_ datasource.DataSourceWithConfigure = &teamDataSource{}
)

type teamDataSource struct {
	client *client.Client
}

func NewTeamDataSource() datasource.DataSource {
	return &teamDataSource{}
}

func (d *teamDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (d *teamDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a team by its exact name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Team name, matched exactly."},
			"id":   schema.StringAttribute{Computed: true, Description: "Team ID."},
			"member_user_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "User IDs of the team's members. Pending invitations are not included.",
			},
		},
	}
}

func (d *teamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *teamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg teamModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teams, err := d.client.ListTeams(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing teams", err.Error())
		return
	}
	t, err := findByName("team", cfg.Name.ValueString(), teams,
		func(t client.Team) string { return t.Name }, func(t client.Team) string { return t.ID })
	if err != nil {
		resp.Diagnostics.AddError("Team lookup failed", err.Error())
		return
	}
	cfg.ID = types.StringValue(t.ID)
	cfg.MemberUserIDs = activeMemberIDs(&t)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
