package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ datasource.DataSource              = &serviceDataSource{}
	_ datasource.DataSourceWithConfigure = &serviceDataSource{}
)

type serviceDataSource struct {
	client *client.Client
}

func NewServiceDataSource() datasource.DataSource {
	return &serviceDataSource{}
}

func (d *serviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *serviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a service by its exact name.",
		Attributes: map[string]schema.Attribute{
			"name":                 schema.StringAttribute{Required: true, Description: "Service name, matched exactly."},
			"id":                   schema.StringAttribute{Computed: true, Description: "Service ID."},
			"description":          schema.StringAttribute{Computed: true, Description: "Free-text description."},
			"escalation_policy_id": schema.StringAttribute{Computed: true, Description: "ID of the escalation policy that pages for this service."},
		},
	}
}

func (d *serviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.Client)
	}
}

func (d *serviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serviceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	services, err := d.client.ListServices(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing services", err.Error())
		return
	}
	svc, err := findByName("service", cfg.Name.ValueString(), services,
		func(s client.Service) string { return s.Name }, func(s client.Service) string { return s.ID })
	if err != nil {
		resp.Diagnostics.AddError("Service lookup failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, serviceFromAPI(&svc))...)
}
