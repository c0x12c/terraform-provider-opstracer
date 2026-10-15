package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &teamResource{}
	_ resource.ResourceWithConfigure   = &teamResource{}
	_ resource.ResourceWithImportState = &teamResource{}
)

type teamResource struct {
	client *client.Client
}

type teamModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	MemberUserIDs []string     `tfsdk:"member_user_ids"`
}

func NewTeamResource() resource.Resource {
	return &teamResource{}
}

func (r *teamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (r *teamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An OpsTracer team.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Team ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Team name.",
				Required:    true,
			},
			"member_user_ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Org member user IDs in the team. Leave unset to manage membership outside Terraform.",
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			},
		},
	}
}

func (r *teamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.CreateTeam(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating team", err.Error())
		return
	}
	plan.ID = types.StringValue(team.ID)
	// Save the ID first so a failed member add does not orphan the team.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)

	resp.Diagnostics.Append(r.syncMembers(ctx, team.ID, plan.MemberUserIDs)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.read(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.GetTeam(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading team", err.Error())
		return
	}
	applyTeam(&state, team)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		if err := r.client.RenameTeam(ctx, state.ID.ValueString(), plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error renaming team", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(r.syncMembers(ctx, state.ID.ValueString(), plan.MemberUserIDs)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(r.read(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteTeam(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting team", err.Error())
	}
}

// Import tracks membership; drop member_user_ids from the config afterwards to leave it unmanaged.
func (r *teamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("member_user_ids"), []string{})...)
}

func (r *teamResource) read(ctx context.Context, m *teamModel) diag.Diagnostics {
	var diags diag.Diagnostics
	team, err := r.client.GetTeam(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Error reading team", err.Error())
		return diags
	}
	applyTeam(m, team)
	return diags
}

// Membership is only reported when the config manages it.
func applyTeam(m *teamModel, t *client.Team) {
	m.Name = types.StringValue(t.Name)
	if m.MemberUserIDs != nil {
		m.MemberUserIDs = activeMemberIDs(t)
	}
}

// syncMembers diffs against live membership, so it also works when state did not track members.
func (r *teamResource) syncMembers(ctx context.Context, teamID string, desired []string) diag.Diagnostics {
	var diags diag.Diagnostics
	if desired == nil {
		return diags
	}
	team, err := r.client.GetTeam(ctx, teamID)
	if err != nil {
		diags.AddError("Error reading team", err.Error())
		return diags
	}
	current := activeMemberIDs(team)
	have := toSet(current)
	var add []string
	for _, id := range desired {
		if !have[id] {
			add = append(add, id)
		}
	}
	if len(add) > 0 {
		if err := r.client.AddTeamMembers(ctx, teamID, add); err != nil {
			diags.AddError("Error adding team members", err.Error())
			return diags
		}
	}
	want := toSet(desired)
	for _, id := range current {
		if !want[id] {
			if err := r.client.RemoveTeamMember(ctx, teamID, id); err != nil && !client.IsNotFound(err) {
				diags.AddError("Error removing team member", err.Error())
				return diags
			}
		}
	}
	return diags
}

func activeMemberIDs(t *client.Team) []string {
	ids := []string{}
	for _, m := range t.Members {
		if m.UserID != nil {
			ids = append(ids, *m.UserID)
		}
	}
	return ids
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
