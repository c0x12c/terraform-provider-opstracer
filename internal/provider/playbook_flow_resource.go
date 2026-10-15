package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &playbookFlowResource{}
	_ resource.ResourceWithConfigure   = &playbookFlowResource{}
	_ resource.ResourceWithImportState = &playbookFlowResource{}
)

type playbookFlowResource struct {
	client *client.Client
}

type playbookFlowModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Visibility  types.String `tfsdk:"visibility"`
	Graph       jsonValue    `tfsdk:"graph"`
	Version     types.Int64  `tfsdk:"version"`
}

func NewPlaybookFlowResource() resource.Resource {
	return &playbookFlowResource{}
}

func (r *playbookFlowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_playbook_flow"
}

func (r *playbookFlowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An investigation playbook flow: a graph of playbooks and conditions that runs when an incident opens.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Playbook flow ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Flow name.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Free-text description.",
				Optional:    true,
			},
			"visibility": schema.StringAttribute{
				Description: "`org` (everyone in the org) or `private` (only the token's user). Defaults to `org`.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("org"),
				Validators:  []validator.String{stringvalidator.OneOf("org", "private")},
			},
			"graph": schema.StringAttribute{
				CustomType: jsonType{},
				Required:   true,
				Description: "Flow graph as JSON, `{nodes = [...], edges = [...]}`; build it with `jsonencode()`. " +
					"Node `type` is `incident`, `playbook`, `condition`, `end` or `topology`; edges use `source_handle` and `target_handle`. " +
					"The building blocks are listed by `GET /api/v1/investigation/playbook-flow/catalog`. " +
					"Key order, whitespace and empty fields (`values = {}`, `playbook = null`) do not cause diffs, " +
					"except right after import, where a `jsonencode()` value that leaves empty fields out matches exactly.",
			},
			"version": schema.Int64Attribute{
				Description: "Server-side revision, bumped on every update.",
				Computed:    true,
			},
		},
	}
}

func (r *playbookFlowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *playbookFlowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan playbookFlowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flow, err := r.client.CreatePlaybookFlow(ctx, playbookFlowInput(plan, 0))
	if err != nil {
		resp.Diagnostics.AddError("Error creating playbook flow", err.Error())
		return
	}
	r.setState(ctx, flow, &resp.State, &resp.Diagnostics)
}

func (r *playbookFlowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state playbookFlowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flow, err := r.client.GetPlaybookFlow(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading playbook flow", err.Error())
		return
	}
	r.setState(ctx, flow, &resp.State, &resp.Diagnostics)
}

func (r *playbookFlowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state playbookFlowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flow, err := r.client.UpdatePlaybookFlow(ctx, playbookFlowInput(plan, state.Version.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error updating playbook flow", err.Error())
		return
	}
	r.setState(ctx, flow, &resp.State, &resp.Diagnostics)
}

func (r *playbookFlowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state playbookFlowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeletePlaybookFlow(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting playbook flow", err.Error())
	}
}

func (r *playbookFlowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type stateSetter interface {
	Set(ctx context.Context, val any) diag.Diagnostics
}

func (r *playbookFlowResource) setState(ctx context.Context, f *client.PlaybookFlow, state stateSetter, diags *diag.Diagnostics) {
	graph, err := canonicalJSON(f.Graph)
	if err != nil {
		diags.AddError("Error decoding playbook flow graph", err.Error())
		return
	}
	diags.Append(state.Set(ctx, playbookFlowModel{
		ID:          types.StringValue(f.ID),
		Name:        types.StringValue(f.Name),
		Description: types.StringPointerValue(f.Description),
		Visibility:  types.StringValue(f.Visibility),
		Graph:       jsonValueOf(graph),
		Version:     types.Int64Value(f.Version),
	})...)
}

func playbookFlowInput(m playbookFlowModel, version int64) client.PlaybookFlowInput {
	return client.PlaybookFlowInput{
		ID:          m.ID.ValueString(),
		Version:     version,
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
		Visibility:  m.Visibility.ValueString(),
		Graph:       json.RawMessage(m.Graph.ValueString()),
	}
}
