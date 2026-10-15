package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &observabilityConnectionResource{}
	_ resource.ResourceWithConfigure   = &observabilityConnectionResource{}
	_ resource.ResourceWithImportState = &observabilityConnectionResource{}
)

type observabilityConnectionResource struct {
	client *client.Client
}

type observabilityConnectionModel struct {
	ID                types.String `tfsdk:"id"`
	Type              types.String `tfsdk:"type"`
	Name              types.String `tfsdk:"name"`
	Site              types.String `tfsdk:"site"`
	APIKey            types.String `tfsdk:"api_key"`
	ApplicationKey    types.String `tfsdk:"application_key"`
	Token             types.String `tfsdk:"token"`
	Organization      types.String `tfsdk:"organization"`
	AccountID         types.String `tfsdk:"account_id"`
	RoleARN           types.String `tfsdk:"role_arn"`
	AccessKeyID       types.String `tfsdk:"access_key_id"`
	SecretAccessKey   types.String `tfsdk:"secret_access_key"`
	ServiceAccountKey types.String `tfsdk:"service_account_key"`
	EnvLabel          types.String `tfsdk:"env_label"`
	IsDefault         types.Bool   `tfsdk:"is_default"`
	Status            types.String `tfsdk:"status"`
	CredentialHints   types.Map    `tfsdk:"credential_hints"`
	Settings          types.Map    `tfsdk:"settings"`
}

func NewObservabilityConnectionResource() resource.Resource {
	return &observabilityConnectionResource{}
}

func (r *observabilityConnectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_observability_connection"
}

func (r *observabilityConnectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Only the name can be changed in place; the API validates credentials once, when it creates the connection.
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	input := func(desc string, sensitive bool) schema.StringAttribute {
		return schema.StringAttribute{
			Description:   desc + " Never returned by the API; changing it replaces the connection. After an import the configured value is adopted in place.",
			Optional:      true,
			Sensitive:     sensitive,
			PlanModifiers: []planmodifier.String{replaceUnlessImported()},
		}
	}
	resp.Schema = schema.Schema{
		Description: "A connection to an observability provider (Datadog, Grafana, Sentry, New Relic, CloudWatch, GCP) that OpsTracer reads " +
			"telemetry from. The API checks the credentials against the provider when the connection is created. Which services use " +
			"a connection is managed with `opstracer_service_data_sources`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Connection ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Description:   "Provider: `datadog`, `grafana`, `sentry`, `newrelic`, `cloudwatch` or `gcp`. Railway connects through OAuth and is not supported here.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf("datadog", "grafana", "sentry", "newrelic", "cloudwatch", "gcp")},
				PlanModifiers: replace,
			},
			"name": schema.StringAttribute{
				Description:   "Connection name. Defaults to the provider's display name, e.g. `Grafana`.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Description:   "Provider site or address, e.g. `datadoghq.eu`, a Grafana stack URL, or an AWS region. Changing it replaces the connection.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured(), stringplanmodifier.UseStateForUnknown()},
			},
			"api_key":             input("API key (Datadog, New Relic).", true),
			"application_key":     input("Application key (Datadog).", true),
			"token":               input("API token (Grafana, Sentry).", true),
			"organization":        input("Organization slug (Sentry).", false),
			"account_id":          input("Account ID (New Relic).", false),
			"role_arn":            input("IAM role ARN to assume (CloudWatch).", false),
			"access_key_id":       input("Access key ID, used instead of `role_arn` (CloudWatch).", false),
			"secret_access_key":   input("Secret access key, used with `access_key_id` (CloudWatch).", true),
			"service_account_key": input("Service account key JSON (GCP).", true),
			"env_label":           input("Label that holds the environment name.", false),
			"is_default": schema.BoolAttribute{
				Description:   "Whether this is the org's default connection for its provider. The first connection of a provider becomes the default.",
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Description:   "Connection status reported by the API, e.g. `active`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"credential_hints": schema.MapAttribute{
				Description:   "Last characters of each stored credential.",
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
			"settings": schema.MapAttribute{
				Description:   "Provider settings the API applied, such as the labels it reads environment and service from.",
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *observabilityConnectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *observabilityConnectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan observabilityConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.client.CreateObservabilityConnection(ctx, client.ObservabilityConnectionInput{
		Type:              plan.Type.ValueString(),
		Name:              knownString(plan.Name),
		Site:              knownString(plan.Site),
		APIKey:            plan.APIKey.ValueStringPointer(),
		ApplicationKey:    plan.ApplicationKey.ValueStringPointer(),
		Token:             plan.Token.ValueStringPointer(),
		Organization:      plan.Organization.ValueStringPointer(),
		AccountID:         plan.AccountID.ValueStringPointer(),
		RoleARN:           plan.RoleARN.ValueStringPointer(),
		AccessKeyID:       plan.AccessKeyID.ValueStringPointer(),
		SecretAccessKey:   plan.SecretAccessKey.ValueStringPointer(),
		ServiceAccountKey: plan.ServiceAccountKey.ValueStringPointer(),
		EnvLabel:          plan.EnvLabel.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating observability connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, observabilityConnectionFromAPI(ctx, conn, plan))...)
}

func (r *observabilityConnectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state observabilityConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.client.GetObservabilityConnection(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading observability connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, observabilityConnectionFromAPI(ctx, conn, state))...)
}

func (r *observabilityConnectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan observabilityConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()
	if !plan.Name.IsUnknown() {
		if err := r.client.RenameObservabilityConnection(ctx, id, plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error renaming observability connection", err.Error())
			return
		}
	}
	conn, err := r.client.GetObservabilityConnection(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading observability connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, observabilityConnectionFromAPI(ctx, conn, plan))...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, nil)...)
}

func (r *observabilityConnectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state observabilityConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RevokeObservabilityConnection(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking observability connection", err.Error())
	}
}

// importedKey marks a connection whose inputs are not in state yet because it was imported.
const importedKey = "imported"

func (r *observabilityConnectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, []byte("true"))...)
}

// replaceUnlessImported lets the first apply after an import adopt the configured inputs in place.
func replaceUnlessImported() planmodifier.String {
	desc := "Changing the value replaces the connection, except when it is first set after an import."
	return stringplanmodifier.RequiresReplaceIf(func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
		imported, diags := req.Private.GetKey(ctx, importedKey)
		resp.Diagnostics.Append(diags...)
		resp.RequiresReplace = !(req.StateValue.IsNull() && imported != nil)
	}, desc, desc)
}

// prior supplies the credentials and other inputs, which the API never returns.
func observabilityConnectionFromAPI(ctx context.Context, c *client.ObservabilityConnection, prior observabilityConnectionModel) observabilityConnectionModel {
	m := prior
	m.ID = types.StringValue(c.ID)
	m.Type = types.StringValue(c.Type)
	m.Name = types.StringValue(c.Name)
	m.Site = types.StringPointerValue(c.Site)
	m.IsDefault = types.BoolValue(c.IsDefault)
	m.Status = types.StringValue(c.Status)
	m.CredentialHints, _ = types.MapValueFrom(ctx, types.StringType, c.CredentialHints)
	m.Settings, _ = types.MapValueFrom(ctx, types.StringType, c.Settings)
	return m
}
