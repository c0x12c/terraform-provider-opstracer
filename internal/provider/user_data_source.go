package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

type userDataSource struct {
	client *client.Client
}

type userModel struct {
	Email       types.String `tfsdk:"email"`
	ID          types.String `tfsdk:"id"`
	DisplayName types.String `tfsdk:"display_name"`
	Role        types.String `tfsdk:"role"`
}

func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an org member by email, to get the user ID for schedules, teams and escalation policies.",
		Attributes: map[string]schema.Attribute{
			"email":        schema.StringAttribute{Required: true},
			"id":           schema.StringAttribute{Computed: true},
			"display_name": schema.StringAttribute{Computed: true},
			"role":         schema.StringAttribute{Computed: true},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg userModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, err := d.client.ListMembers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing org members", err.Error())
		return
	}
	for _, m := range members {
		if strings.EqualFold(m.Email, cfg.Email.ValueString()) {
			cfg.ID = types.StringValue(m.UserID)
			cfg.DisplayName = types.StringPointerValue(m.DisplayName)
			cfg.Role = types.StringValue(m.Role)
			resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
			return
		}
	}
	resp.Diagnostics.AddError("User not found", "No org member has email "+cfg.Email.ValueString())
}
