package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = &orgMemberResource{}
	_ resource.ResourceWithConfigure   = &orgMemberResource{}
	_ resource.ResourceWithImportState = &orgMemberResource{}
)

type orgMemberResource struct {
	client *client.Client
}

type orgMemberModel struct {
	ID          types.String `tfsdk:"id"`
	UserID      types.String `tfsdk:"user_id"`
	Role        types.String `tfsdk:"role"`
	TitleID     types.String `tfsdk:"title_id"`
	Email       types.String `tfsdk:"email"`
	DisplayName types.String `tfsdk:"display_name"`
}

func NewOrgMemberResource() resource.Resource {
	return &orgMemberResource{}
}

func (r *orgMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_member"
}

func (r *orgMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Role and title of an existing org member. Creating it adopts the member (people join through " +
			"`opstracer_org_invitation`); destroying it only drops it from state and never removes the member from the org.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Same as `user_id`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Description:   "User ID of the member, e.g. from the `opstracer_user` data source.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"role": schema.StringAttribute{
				Description:   "Org role: `OWNER`, `ADMIN` or `MEMBER`. Left as it is when unset.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf("OWNER", "ADMIN", "MEMBER")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"title_id": schema.StringAttribute{
				Description: "Member title ID, built-in or from `opstracer_member_title`. Unset means no title.",
				Optional:    true,
			},
			"email": schema.StringAttribute{
				Description:   "Member email.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				Description:   "Member display name.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *orgMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *orgMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, diags := r.apply(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	}
}

func (r *orgMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.findMember(ctx, state.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading org member", err.Error())
		return
	}
	if m == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, memberFromAPI(m))...)
}

func (r *orgMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan orgMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, diags := r.apply(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	}
}

// apply calls only the endpoints whose value differs from the server, so an unset role is never sent.
func (r *orgMemberResource) apply(ctx context.Context, plan orgMemberModel) (orgMemberModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	userID := plan.UserID.ValueString()
	m, err := r.findMember(ctx, userID)
	if err != nil {
		diags.AddError("Error reading org member", err.Error())
		return plan, diags
	}
	if m == nil {
		diags.AddError("Member not found", "User "+userID+" is not a member of the org.")
		return plan, diags
	}
	if role := knownString(plan.Role); role != nil && *role != m.Role {
		if m, err = r.client.UpdateMemberRole(ctx, userID, *role); err != nil {
			diags.AddError("Error updating member role", err.Error())
			return plan, diags
		}
	}
	if title := plan.TitleID.ValueStringPointer(); !sameStringPtr(title, memberTitleID(m)) {
		if m, err = r.client.AssignMemberTitle(ctx, userID, title); err != nil {
			diags.AddError("Error updating member title", err.Error())
			return plan, diags
		}
	}
	return memberFromAPI(m), diags
}

func (r *orgMemberResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *orgMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("user_id"), req, resp)
}

func (r *orgMemberResource) findMember(ctx context.Context, userID string) (*client.Member, error) {
	members, err := r.client.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		if m.UserID == userID {
			return &m, nil
		}
	}
	return nil, nil
}

func memberFromAPI(m *client.Member) orgMemberModel {
	return orgMemberModel{
		ID:          types.StringValue(m.UserID),
		UserID:      types.StringValue(m.UserID),
		Role:        types.StringValue(m.Role),
		TitleID:     types.StringPointerValue(memberTitleID(m)),
		Email:       types.StringValue(m.Email),
		DisplayName: types.StringPointerValue(m.DisplayName),
	}
}

func memberTitleID(m *client.Member) *string {
	if m.Title == nil {
		return nil
	}
	return &m.Title.ID
}

func sameStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
