package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource              = &scheduleDataSource{}
	_ datasource.DataSourceWithConfigure = &scheduleDataSource{}
)

type scheduleDataSource struct {
	client *client.Client
}

type scheduleLookupModel struct {
	Name     types.String `tfsdk:"name"`
	ID       types.String `tfsdk:"id"`
	TimeZone types.String `tfsdk:"time_zone"`
}

func NewScheduleDataSource() datasource.DataSource {
	return &scheduleDataSource{}
}

func (d *scheduleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (d *scheduleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an on-call schedule by its exact name.",
		Attributes: map[string]schema.Attribute{
			"name":      schema.StringAttribute{Required: true, Description: "Schedule name, matched exactly."},
			"id":        schema.StringAttribute{Computed: true, Description: "Schedule ID."},
			"time_zone": schema.StringAttribute{Computed: true, Description: "IANA time zone of the schedule."},
		},
	}
}

func (d *scheduleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *scheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg scheduleLookupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schedules, err := d.client.ListSchedules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing schedules", err.Error())
		return
	}
	s, err := findByName("schedule", cfg.Name.ValueString(), schedules,
		func(s client.Schedule) string { return s.Name }, func(s client.Schedule) string { return s.ID })
	if err != nil {
		resp.Diagnostics.AddError("Schedule lookup failed", err.Error())
		return
	}
	cfg.ID = types.StringValue(s.ID)
	cfg.TimeZone = types.StringValue(s.TimeZone)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
