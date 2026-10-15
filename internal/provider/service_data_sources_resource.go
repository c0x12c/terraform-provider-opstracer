package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                   = &serviceDataSourcesResource{}
	_ resource.ResourceWithConfigure      = &serviceDataSourcesResource{}
	_ resource.ResourceWithImportState    = &serviceDataSourcesResource{}
	_ resource.ResourceWithValidateConfig = &serviceDataSourcesResource{}
)

const choiceDefault = "default"

type serviceDataSourcesResource struct {
	client *client.Client
}

type serviceDataSourcesModel struct {
	ID        types.String                 `tfsdk:"id"`
	ServiceID types.String                 `tfsdk:"service_id"`
	Sources   map[string]serviceDataSource `tfsdk:"sources"`
}

type serviceDataSource struct {
	Choice       types.String `tfsdk:"choice"`
	ConnectionID types.String `tfsdk:"connection_id"`
}

func NewServiceDataSourcesResource() resource.Resource {
	return &serviceDataSourcesResource{}
}

func (r *serviceDataSourcesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_data_sources"
}

func (r *serviceDataSourcesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Which observability connection a service reads from, per provider. Providers not listed keep the org default " +
			"connection. Destroying the resource puts every listed provider back to the default.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Same as `service_id`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Description:   "ID of the service.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sources": schema.MapNestedAttribute{
				Description: "Choices keyed by provider type (`datadog`, `grafana`, `sentry`, `newrelic`, `cloudwatch`, `gcp`, `railway`).",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"choice": schema.StringAttribute{
							// Enforced in ValidateConfig: Required here breaks on an element object that is unknown at validate time.
							Description: "Required. `default` (the org default connection), `connection` (the one in `connection_id`) or `off` (do not read this provider).",
							Optional:    true,
							Validators:  []validator.String{stringvalidator.OneOf(choiceDefault, "connection", "off")},
						},
						"connection_id": schema.StringAttribute{
							Description: "Observability connection to read from. Required when `choice` is `connection`.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *serviceDataSourcesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *serviceDataSourcesResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var raw types.Map
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("sources"), &raw)...)
	if resp.Diagnostics.HasError() || raw.IsNull() || raw.IsUnknown() {
		return
	}
	var sources map[string]types.Object
	resp.Diagnostics.Append(raw.ElementsAs(ctx, &sources, false)...)
	for typ, obj := range sources {
		var s serviceDataSource
		if obj.IsUnknown() || obj.As(ctx, &s, basetypes.ObjectAsOptions{}).HasError() || s.Choice.IsUnknown() || s.ConnectionID.IsUnknown() {
			continue
		}
		if s.Choice.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("sources").AtMapKey(typ).AtName("choice"), "Missing choice", "Set choice for every provider.")
			continue
		}
		if (s.Choice.ValueString() == "connection") != !s.ConnectionID.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("sources").AtMapKey(typ),
				"Invalid data source", "Set connection_id exactly when choice is \"connection\".")
		}
	}
}

func (r *serviceDataSourcesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceDataSourcesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, nil, resp.Diagnostics.AddError, func(m serviceDataSourcesModel) { resp.Diagnostics.Append(resp.State.Set(ctx, m)...) })
}

func (r *serviceDataSourcesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceDataSourcesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.read(ctx, state)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading service data sources", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *serviceDataSourcesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state serviceDataSourcesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, state.Sources, resp.Diagnostics.AddError, func(m serviceDataSourcesModel) { resp.Diagnostics.Append(resp.State.Set(ctx, m)...) })
}

func (r *serviceDataSourcesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceDataSourcesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for typ := range state.Sources {
		err := r.client.SetServiceDataSource(ctx, client.ServiceDataSourceInput{ServiceID: state.ServiceID.ValueString(), Type: typ, Choice: choiceDefault})
		if err != nil && !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error resetting service data source "+typ, err.Error())
			return
		}
	}
}

func (r *serviceDataSourcesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("service_id"), req, resp)
}

// apply sets every planned choice and resets providers dropped from the config to the default.
func (r *serviceDataSourcesResource) apply(ctx context.Context, plan serviceDataSourcesModel, prior map[string]serviceDataSource,
	addError func(string, string), setState func(serviceDataSourcesModel)) {
	serviceID := plan.ServiceID.ValueString()
	for typ := range prior {
		if _, ok := plan.Sources[typ]; ok {
			continue
		}
		if err := r.client.SetServiceDataSource(ctx, client.ServiceDataSourceInput{ServiceID: serviceID, Type: typ, Choice: choiceDefault}); err != nil {
			addError("Error resetting service data source "+typ, err.Error())
			return
		}
	}
	for typ, s := range plan.Sources {
		err := r.client.SetServiceDataSource(ctx, client.ServiceDataSourceInput{
			ServiceID:    serviceID,
			Type:         typ,
			Choice:       s.Choice.ValueString(),
			ConnectionID: s.ConnectionID.ValueStringPointer(),
		})
		if err != nil {
			addError("Error setting service data source "+typ, err.Error())
			return
		}
	}

	m, err := r.read(ctx, plan)
	if err != nil {
		addError("Error reading service data sources", err.Error())
		return
	}
	setState(m)
}

// read returns the API's non-default choices plus every provider already tracked in prior, so an
// explicit `default` stays in state while overrides made outside Terraform still show up as drift.
func (r *serviceDataSourcesResource) read(ctx context.Context, prior serviceDataSourcesModel) (serviceDataSourcesModel, error) {
	serviceID := prior.ServiceID.ValueString()
	sources, err := r.client.GetServiceDataSources(ctx, serviceID)
	if err != nil {
		return prior, err
	}
	m := serviceDataSourcesModel{
		ID:        types.StringValue(serviceID),
		ServiceID: types.StringValue(serviceID),
		Sources:   map[string]serviceDataSource{},
	}
	for typ := range prior.Sources {
		m.Sources[typ] = serviceDataSource{Choice: types.StringValue(choiceDefault), ConnectionID: types.StringNull()}
	}
	for _, s := range sources {
		if _, tracked := prior.Sources[s.Type]; tracked || s.Choice != choiceDefault {
			m.Sources[s.Type] = serviceDataSource{Choice: types.StringValue(s.Choice), ConnectionID: types.StringPointerValue(s.ConnectionID)}
		}
	}
	return m, nil
}
