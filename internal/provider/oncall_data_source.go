package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource              = &onCallDataSource{}
	_ datasource.DataSourceWithConfigure = &onCallDataSource{}
)

type onCallDataSource struct {
	client *client.Client
}

type onCallModel struct {
	ScheduleID types.String `tfsdk:"schedule_id"`
	At         types.String `tfsdk:"at"`
	UserIDs    []string     `tfsdk:"user_ids"`
	Until      types.String `tfsdk:"until"`
}

func NewOnCallDataSource() datasource.DataSource {
	return &onCallDataSource{}
}

func (d *onCallDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall"
}

func (d *onCallDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Who is on call for a schedule, now or at a given time. Re-read on every plan, so values change as shifts rotate.",
		Attributes: map[string]schema.Attribute{
			"schedule_id": schema.StringAttribute{Required: true, Description: "Schedule ID."},
			"at": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "RFC 3339 time to evaluate. Defaults to now; the server's evaluation time is stored here.",
			},
			"user_ids": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "User IDs on call at `at`. Empty when the shift is uncovered.",
			},
			"until": schema.StringAttribute{Computed: true, Description: "End of the current shift (RFC 3339, UTC), if known."},
		},
	}
}

func (d *onCallDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *onCallDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg onCallModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oc, err := d.client.GetOnCall(ctx, cfg.ScheduleID.ValueString(), cfg.At.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading on-call", err.Error())
		return
	}
	if cfg.At.IsNull() {
		cfg.At = types.StringValue(oc.At)
	}
	cfg.UserIDs = oc.UserIDs
	if cfg.UserIDs == nil {
		cfg.UserIDs = []string{}
	}
	cfg.Until = types.StringPointerValue(oc.Until)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
