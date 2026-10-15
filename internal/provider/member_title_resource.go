package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &memberTitleResource{}
	_ resource.ResourceWithConfigure   = &memberTitleResource{}
	_ resource.ResourceWithImportState = &memberTitleResource{}
)

type memberTitleResource struct {
	client *client.Client
}

type memberTitleModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewMemberTitleResource() resource.Resource {
	return &memberTitleResource{}
}

func (r *memberTitleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member_title"
}

func (r *memberTitleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A custom member title for the org, alongside the built-in ones (Engineer, Manager, ...).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *memberTitleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *memberTitleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberTitleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	t, err := r.client.CreateMemberTitle(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating member title", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, memberTitleModel{ID: types.StringValue(t.ID), Name: types.StringValue(t.Name)})...)
}

// There is no get-by-id endpoint, so Read scans the list.
func (r *memberTitleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberTitleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titles, err := r.client.ListMemberTitles(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading member titles", err.Error())
		return
	}
	for _, t := range titles {
		if t.ID == state.ID.ValueString() {
			state.Name = types.StringValue(t.Name)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *memberTitleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan memberTitleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	t, err := r.client.UpdateMemberTitle(ctx, plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating member title", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, memberTitleModel{ID: types.StringValue(t.ID), Name: types.StringValue(t.Name)})...)
}

func (r *memberTitleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberTitleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteMemberTitle(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting member title", err.Error())
	}
}

func (r *memberTitleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
