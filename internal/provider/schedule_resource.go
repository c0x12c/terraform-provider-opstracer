package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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
	_ resource.Resource                = &scheduleResource{}
	_ resource.ResourceWithConfigure   = &scheduleResource{}
	_ resource.ResourceWithImportState = &scheduleResource{}
)

type scheduleResource struct {
	client *client.Client
}

type scheduleModel struct {
	ID           types.String          `tfsdk:"id"`
	Name         types.String          `tfsdk:"name"`
	Description  types.String          `tfsdk:"description"`
	TimeZone     types.String          `tfsdk:"time_zone"`
	Layers       []scheduleLayer       `tfsdk:"layers"`
	Announcement *scheduleAnnouncement `tfsdk:"announcement"`
}

type scheduleAnnouncement struct {
	SlackChannelID   types.String `tfsdk:"slack_channel_id"`
	SlackChannelName types.String `tfsdk:"slack_channel_name"`
}

type scheduleLayer struct {
	Name                types.String       `tfsdk:"name"`
	TimeZone            types.String       `tfsdk:"time_zone"`
	RotationStartAt     types.String       `tfsdk:"rotation_start_at"`
	MemberPeriodSeconds types.Int64        `tfsdk:"member_period_seconds"`
	ParticipantGroups   [][]string         `tfsdk:"participant_groups"`
	Restrictions        []layerRestriction `tfsdk:"restrictions"`
}

type layerRestriction struct {
	Days            []int64     `tfsdk:"days"`
	StartTimeOfDay  types.Int64 `tfsdk:"start_time_of_day"`
	DurationSeconds types.Int64 `tfsdk:"duration_seconds"`
}

func NewScheduleResource() resource.Resource {
	return &scheduleResource{}
}

func (r *scheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (r *scheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An on-call schedule. Each layer is a rotation; when layers overlap, the later layer wins.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Schedule ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Schedule name.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Free-text description.",
				Optional:    true,
			},
			"time_zone": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "IANA time zone, e.g. `Asia/Ho_Chi_Minh`. Defaults to the org time zone.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"announcement": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Post shift changes to a Slack channel. Needs the Slack integration connected. Unset turns announcements off.",
				Attributes: map[string]schema.Attribute{
					"slack_channel_id": schema.StringAttribute{
						Required:    true,
						Description: "Slack channel ID, e.g. `C0123456789`.",
					},
					"slack_channel_name": schema.StringAttribute{
						Optional:    true,
						Description: "Channel name shown in the web app.",
					},
				},
			},
			"layers": schema.ListNestedAttribute{
				Required:    true,
				Description: "Rotations, in order.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Layer name.",
							Optional:    true,
						},
						"time_zone": schema.StringAttribute{
							Optional:      true,
							Computed:      true,
							Description:   "Defaults to the schedule time zone.",
							PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"rotation_start_at": schema.StringAttribute{
							Required:    true,
							Description: "RFC 3339 timestamp when the first turn starts, e.g. `2026-10-14T09:00:00+07:00`.",
							Validators:  []validator.String{rfc3339Validator{}},
						},
						"member_period_seconds": schema.Int64Attribute{
							Required:    true,
							Description: "Length of one turn. 86400 = daily, 604800 = weekly.",
							Validators:  []validator.Int64{int64validator.AtLeast(1)},
						},
						"participant_groups": schema.ListAttribute{
							Required:    true,
							ElementType: types.ListType{ElemType: types.StringType},
							Description: "Turns in rotation order. Each turn is a list of user IDs on call together.",
							Validators: []validator.List{
								listvalidator.SizeAtLeast(1),
								listvalidator.ValueListsAre(listvalidator.SizeAtLeast(1)),
							},
						},
						"restrictions": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Limit the layer to these windows. Leave unset for around-the-clock coverage.",
							Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"days": schema.SetAttribute{
										Required:    true,
										ElementType: types.Int64Type,
										Description: "ISO weekdays: 1 = Monday ... 7 = Sunday.",
										Validators: []validator.Set{
											setvalidator.SizeAtLeast(1),
											setvalidator.ValueInt64sAre(int64validator.Between(1, 7)),
										},
									},
									"start_time_of_day": schema.Int64Attribute{
										Required:    true,
										Description: "Seconds after midnight in the layer time zone. 32400 = 09:00.",
										Validators:  []validator.Int64{int64validator.Between(0, 86399)},
									},
									"duration_seconds": schema.Int64Attribute{
										Description: "Length of the window in seconds. 28800 = 8 hours.",
										Required:    true,
										Validators:  []validator.Int64{int64validator.Between(1, 86400)},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *scheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *scheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.CreateSchedule(ctx, scheduleInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating schedule", err.Error())
		return
	}
	state := scheduleFromAPI(s, plan)
	// Save the schedule first so a failed announcement does not orphan it.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	if plan.Announcement != nil {
		if state.Announcement, err = r.setAnnouncement(ctx, s.ID, plan.Announcement); err != nil {
			resp.Diagnostics.AddError("Error setting schedule announcement", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	}
}

func (r *scheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.GetSchedule(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading schedule", err.Error())
		return
	}
	a, err := r.client.GetScheduleAnnouncement(ctx, s.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading schedule announcement", err.Error())
		return
	}
	m := scheduleFromAPI(s, state)
	m.Announcement = announcementFromAPI(a, state.Announcement)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *scheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := r.client.UpdateSchedule(ctx, scheduleInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating schedule", err.Error())
		return
	}
	state := scheduleFromAPI(s, plan)
	state.Announcement = prior.Announcement
	if !sameAnnouncement(plan.Announcement, prior.Announcement) {
		if state.Announcement, err = r.setAnnouncement(ctx, s.ID, plan.Announcement); err != nil {
			resp.Diagnostics.AddError("Error updating schedule announcement", err.Error())
			state.Announcement = prior.Announcement
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *scheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSchedule(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting schedule", err.Error())
	}
}

func (r *scheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *scheduleResource) setAnnouncement(ctx context.Context, scheduleID string, a *scheduleAnnouncement) (*scheduleAnnouncement, error) {
	in := client.ScheduleAnnouncement{ScheduleID: scheduleID}
	if a != nil {
		in.SlackChannelID = a.SlackChannelID.ValueStringPointer()
		in.SlackChannelName = a.SlackChannelName.ValueStringPointer()
	}
	out, err := r.client.UpdateScheduleAnnouncement(ctx, in)
	if err != nil {
		return nil, err
	}
	return announcementFromAPI(out, a), nil
}

// announcementFromAPI returns nil when no channel is set; prior fills in a channel name the server does not echo.
func announcementFromAPI(a *client.ScheduleAnnouncement, prior *scheduleAnnouncement) *scheduleAnnouncement {
	if a.SlackChannelID == nil || *a.SlackChannelID == "" {
		return nil
	}
	m := &scheduleAnnouncement{
		SlackChannelID:   types.StringPointerValue(a.SlackChannelID),
		SlackChannelName: types.StringPointerValue(a.SlackChannelName),
	}
	if a.SlackChannelName == nil && prior != nil && prior.SlackChannelID.Equal(m.SlackChannelID) {
		m.SlackChannelName = prior.SlackChannelName
	}
	return m
}

func sameAnnouncement(a, b *scheduleAnnouncement) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.SlackChannelID.Equal(b.SlackChannelID) && a.SlackChannelName.Equal(b.SlackChannelName)
}

func scheduleInput(m scheduleModel) client.ScheduleInput {
	in := client.ScheduleInput{
		ID:          m.ID.ValueString(),
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueStringPointer(),
		TimeZone:    knownString(m.TimeZone),
	}
	seen := map[string]bool{}
	for _, l := range m.Layers {
		layer := client.ScheduleLayer{
			Name:                l.Name.ValueStringPointer(),
			TimeZone:            knownString(l.TimeZone),
			RotationStartAt:     l.RotationStartAt.ValueString(),
			MemberPeriodSeconds: l.MemberPeriodSeconds.ValueInt64(),
			ParticipantGroups:   l.ParticipantGroups,
			Restrictions:        []client.LayerRestriction{},
		}
		for _, g := range l.ParticipantGroups {
			for _, id := range g {
				layer.ParticipantUserIDs = appendUnique(layer.ParticipantUserIDs, id)
				if !seen[id] {
					seen[id] = true
					in.ParticipantUserIDs = append(in.ParticipantUserIDs, id)
				}
			}
		}
		for _, r := range l.Restrictions {
			days := append([]int64(nil), r.Days...)
			sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
			layer.Restrictions = append(layer.Restrictions, client.LayerRestriction{
				Days:            days,
				StartTimeOfDay:  r.StartTimeOfDay.ValueInt64(),
				DurationSeconds: r.DurationSeconds.ValueInt64(),
			})
		}
		in.Layers = append(in.Layers, layer)
	}
	if len(in.Layers) > 0 {
		in.RotationStartAt = in.Layers[0].RotationStartAt
		in.MemberPeriodSeconds = in.Layers[0].MemberPeriodSeconds
	}
	return in
}

// scheduleFromAPI keeps prior's rotation_start_at spelling when it is the same instant the server returned.
func scheduleFromAPI(s *client.Schedule, prior scheduleModel) scheduleModel {
	m := scheduleModel{
		ID:          types.StringValue(s.ID),
		Name:        types.StringValue(s.Name),
		Description: types.StringPointerValue(s.Description),
		TimeZone:    types.StringValue(s.TimeZone),
	}
	for i, l := range s.Layers {
		layer := scheduleLayer{
			Name:                types.StringPointerValue(l.Name),
			TimeZone:            types.StringPointerValue(l.TimeZone),
			RotationStartAt:     types.StringValue(l.RotationStartAt),
			MemberPeriodSeconds: types.Int64Value(l.MemberPeriodSeconds),
			ParticipantGroups:   l.ParticipantGroups,
		}
		if layer.ParticipantGroups == nil {
			layer.ParticipantGroups = [][]string{}
		}
		if i < len(prior.Layers) && sameInstant(prior.Layers[i].RotationStartAt.ValueString(), l.RotationStartAt) {
			layer.RotationStartAt = prior.Layers[i].RotationStartAt
		}
		for _, r := range l.Restrictions {
			layer.Restrictions = append(layer.Restrictions, layerRestriction{
				Days:            r.Days,
				StartTimeOfDay:  types.Int64Value(r.StartTimeOfDay),
				DurationSeconds: types.Int64Value(r.DurationSeconds),
			})
		}
		m.Layers = append(m.Layers, layer)
	}
	return m
}

func sameInstant(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func knownString(v types.String) *string {
	if v.IsUnknown() {
		return nil
	}
	return v.ValueStringPointer()
}

type rfc3339Validator struct{}

func (rfc3339Validator) Description(context.Context) string {
	return "must be an RFC 3339 timestamp"
}

func (v rfc3339Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (rfc3339Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.Parse(time.RFC3339, req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid timestamp",
			fmt.Sprintf("%q is not RFC 3339, e.g. 2026-10-14T09:00:00+07:00", req.ConfigValue.ValueString()))
	}
}
