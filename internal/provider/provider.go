package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

const defaultAPIURL = "https://api-control-plane.opstracer.ai"

var _ provider.Provider = &opstracerProvider{}

type opstracerProvider struct {
	version string
}

type providerModel struct {
	APIURL types.String `tfsdk:"api_url"`
	Token  types.String `tfsdk:"token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &opstracerProvider{version: version}
	}
}

func (p *opstracerProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "opstracer"
	resp.Version = p.version
}

func (p *opstracerProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage OpsTracer on-call configuration.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional:    true,
				Description: "OpsTracer control-plane API URL. Falls back to OPSTRACER_API_URL, then " + defaultAPIURL + ".",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Org-scoped bearer token. Falls back to OPSTRACER_TOKEN.",
			},
		},
	}
}

func (p *opstracerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := firstNonEmpty(cfg.APIURL.ValueString(), os.Getenv("OPSTRACER_API_URL"), defaultAPIURL)
	token := firstNonEmpty(cfg.Token.ValueString(), os.Getenv("OPSTRACER_TOKEN"))
	if token == "" {
		resp.Diagnostics.AddError("Missing OpsTracer token", "Set the provider `token` argument or the OPSTRACER_TOKEN environment variable.")
		return
	}

	c := client.New(apiURL, token, "terraform-provider-opstracer/"+p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *opstracerProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServiceResource,
		NewEscalationPolicyResource,
		NewScheduleResource,
		NewTeamResource,
		NewRoutingRulesResource,
		NewNotificationOrderResource,
		NewMemberTitleResource,
		NewOrgInvitationResource,
		NewOrgMemberResource,
		NewScheduleOverrideResource,
	}
}

func (p *opstracerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewUserDataSource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
