package provider

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

var (
	_ resource.Resource                = &orgInvitationResource{}
	_ resource.ResourceWithConfigure   = &orgInvitationResource{}
	_ resource.ResourceWithImportState = &orgInvitationResource{}
	_ resource.ResourceWithModifyPlan  = &orgInvitationResource{}
)

const invitationAccepted = "accepted"

type orgInvitationResource struct {
	client *client.Client
}

type orgInvitationModel struct {
	ID            types.String `tfsdk:"id"`
	Email         types.String `tfsdk:"email"`
	Role          types.String `tfsdk:"role"`
	TeamIDs       []string     `tfsdk:"team_ids"`
	SendEmail     types.Bool   `tfsdk:"send_email"`
	ResendTrigger types.String `tfsdk:"resend_trigger"`
	Status        types.String `tfsdk:"status"`
	InviteURL     types.String `tfsdk:"invite_url"`
}

func NewOrgInvitationResource() resource.Resource {
	return &orgInvitationResource{}
}

func (r *orgInvitationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_invitation"
}

func (r *orgInvitationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An invitation for someone to join the org. Once the invitee accepts, the invitation stays in state " +
			"with `status = \"accepted\"` and destroying it only drops it from state; manage the new member with " +
			"`opstracer_org_member`. A revoked or expired invitation is removed from state and planned for re-creation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Invitation ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email": schema.StringAttribute{
				Description:   "Email address to invite. Compared case-insensitively.",
				Required:      true,
				PlanModifiers: []planmodifier.String{caseInsensitive{}, stringplanmodifier.RequiresReplace()},
			},
			"role": schema.StringAttribute{
				Description: "Org role the invitee gets: `ADMIN` or `MEMBER`. Can be changed while the invitation is pending.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.OneOf("ADMIN", "MEMBER")},
			},
			"team_ids": schema.SetAttribute{
				Description: "Teams the invitee joins on accepting. Can be changed while the invitation is pending.",
				Optional:    true,
				ElementType: types.StringType,
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"send_email": schema.BoolAttribute{
				Description: "Email the invitation on create and on resend. Set `false` to share `invite_url` yourself.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"resend_trigger": schema.StringAttribute{
				Description: "Any value. Changing it from one value to another resends the invitation, which also issues a new `invite_url`.",
				Optional:    true,
			},
			"status": schema.StringAttribute{
				Description:   "`pending`, or `accepted` once the invitee has joined.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"invite_url": schema.StringAttribute{
				Description: "Join link. Only returned when the invitation is created or resent, so it is empty after import. " +
					"Changing `role`, `team_ids` or `resend_trigger` issues a new link.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

// ModifyPlan keeps invite_url (possibly null after import) unless the update will issue a new link.
func (r *orgInvitationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	// team_ids may still be unknown here, so read attributes as framework values.
	var planRole, stateRole, planTrigger, stateTrigger types.String
	var planTeams, stateTeams types.Set
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("role"), &planRole)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("role"), &stateRole)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("resend_trigger"), &planTrigger)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("resend_trigger"), &stateTrigger)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("team_ids"), &planTeams)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("team_ids"), &stateTeams)...)
	if resp.Diagnostics.HasError() || !planRole.Equal(stateRole) || !planTeams.Equal(stateTeams) || resends(planTrigger, stateTrigger) {
		return
	}
	var url types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("invite_url"), &url)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("invite_url"), url)...)
}

// resends reports a change of resend_trigger; setting it for the first time (e.g. after import) does not count.
func resends(plan, state types.String) bool {
	return !state.IsNull() && !plan.Equal(state)
}

func (r *orgInvitationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *orgInvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgInvitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inv, err := r.client.CreateInvitation(ctx, invitationInput(plan, plan.SendEmail.ValueBool()))
	if err != nil {
		resp.Diagnostics.AddError("Error creating invitation", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, invitationFromAPI(inv, plan))...)
}

func (r *orgInvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgInvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.Status.ValueString() == invitationAccepted {
		return
	}

	// No get-by-id endpoint: scan the pending list.
	pending, err := r.client.ListPendingInvitations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing invitations", err.Error())
		return
	}
	for _, inv := range pending {
		if inv.ID == state.ID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, invitationFromAPI(&inv, state))...)
			return
		}
	}

	// Accepted invitations drop out of the list; the invitee then shows up as a member.
	if !state.Email.IsNull() {
		members, err := r.client.ListMembers(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing org members", err.Error())
			return
		}
		for _, m := range members {
			if strings.EqualFold(m.Email, state.Email.ValueString()) {
				state.Status = types.StringValue(invitationAccepted)
				resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
				return
			}
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *orgInvitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgInvitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result := state
	result.SendEmail = plan.SendEmail
	result.ResendTrigger = plan.ResendTrigger
	if state.Status.ValueString() == invitationAccepted {
		resp.Diagnostics.AddWarning("Invitation already accepted",
			"Changes to an accepted invitation have no effect. Use opstracer_org_member to manage the member.")
		resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
		return
	}

	if !plan.Role.Equal(state.Role) || !sameStrings(plan.TeamIDs, state.TeamIDs) {
		// Creating again for the same email updates the pending invitation in place.
		inv, err := r.client.CreateInvitation(ctx, invitationInput(plan, false))
		if err != nil {
			resp.Diagnostics.AddError("Error updating invitation", err.Error())
			return
		}
		result = invitationFromAPI(inv, plan)
	}
	if resends(plan.ResendTrigger, state.ResendTrigger) {
		inv, err := r.client.ResendInvitation(ctx, result.ID.ValueString(), plan.SendEmail.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Error resending invitation", err.Error())
			return
		}
		result = invitationFromAPI(inv, plan)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *orgInvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgInvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.Status.ValueString() == invitationAccepted {
		return
	}

	err := r.client.RevokeInvitation(ctx, state.ID.ValueString())
	if err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Error revoking invitation", err.Error())
	}
}

func (r *orgInvitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("send_email"), true)...)
}

func invitationInput(m orgInvitationModel, sendEmail bool) client.InvitationInput {
	return client.InvitationInput{
		Email:     m.Email.ValueString(),
		Role:      m.Role.ValueString(),
		TeamIDs:   m.TeamIDs,
		SendEmail: sendEmail,
	}
}

// invitationFromAPI keeps prior's email spelling (the server lowercases it) and invite_url when the response has none.
func invitationFromAPI(inv *client.Invitation, prior orgInvitationModel) orgInvitationModel {
	m := orgInvitationModel{
		ID:            types.StringValue(inv.ID),
		Email:         types.StringValue(inv.Email),
		Role:          types.StringValue(inv.Role),
		SendEmail:     prior.SendEmail,
		ResendTrigger: prior.ResendTrigger,
		Status:        types.StringValue(inv.Status),
		InviteURL:     types.StringPointerValue(inv.InviteURL),
	}
	if strings.EqualFold(prior.Email.ValueString(), inv.Email) {
		m.Email = prior.Email
	}
	if inv.InviteURL == nil && !prior.InviteURL.IsUnknown() {
		m.InviteURL = prior.InviteURL
	}
	for _, t := range inv.Teams {
		m.TeamIDs = append(m.TeamIDs, t.ID)
	}
	return m
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// caseInsensitive keeps the state value when the planned one differs only in case.
type caseInsensitive struct{}

func (caseInsensitive) Description(context.Context) string {
	return "Differences in case only are ignored."
}

func (m caseInsensitive) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (caseInsensitive) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}
	if strings.EqualFold(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}
