package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

const notificationOrderID = "notification_order"

var (
	_ resource.Resource                = &notificationOrderResource{}
	_ resource.ResourceWithConfigure   = &notificationOrderResource{}
	_ resource.ResourceWithImportState = &notificationOrderResource{}
)

type notificationOrderResource struct {
	client *client.Client
}

type notificationOrderModel struct {
	ID      types.String       `tfsdk:"id"`
	Enabled types.Bool         `tfsdk:"enabled"`
	Tiers   []notificationTier `tfsdk:"tiers"`
}

type notificationTier struct {
	Channels     []string    `tfsdk:"channels"`
	DelaySeconds types.Int64 `tfsdk:"delay_seconds"`
}

func NewNotificationOrderResource() resource.Resource {
	return &notificationOrderResource{}
}

func (r *notificationOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_order"
}

func (r *notificationOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The org-wide order in which channels are tried when paging a responder. One per org; " +
			"destroying it only removes it from state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Required: true,
			},
			"tiers": schema.ListNestedAttribute{
				Required:    true,
				Description: "Tried in order. Each channel may appear once across all tiers.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channels": schema.ListAttribute{
							Required:    true,
							ElementType: types.StringType,
							Description: "e.g. `slack`, `email`, `telegram`, `push`, `sms`, `voice`.",
							Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
						},
						"delay_seconds": schema.Int64Attribute{
							Optional:   true,
							Computed:   true,
							Default:    int64default.StaticInt64(0),
							Validators: []validator.Int64{int64validator.Between(0, 900)},
						},
					},
				},
			},
		},
	}
}

func (r *notificationOrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.Client)
	}
}

func (r *notificationOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationOrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, diags := r.save(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if !diags.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	}
}

func (r *notificationOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationOrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, diags := r.save(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if !diags.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	}
}

func (r *notificationOrderResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	o, err := r.client.GetNotificationOrder(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading notification order", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, notificationOrderFromAPI(o))...)
}

func (r *notificationOrderResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *notificationOrderResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), notificationOrderID)...)
}

func (r *notificationOrderResource) save(ctx context.Context, plan notificationOrderModel) (notificationOrderModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	in := client.NotificationOrder{Enabled: plan.Enabled.ValueBool()}
	for _, t := range plan.Tiers {
		in.Tiers = append(in.Tiers, client.NotificationTier{Channels: t.Channels, DelaySeconds: t.DelaySeconds.ValueInt64()})
	}
	o, err := r.client.SetNotificationOrder(ctx, in)
	if err != nil {
		diags.AddError("Error saving notification order", err.Error())
		return plan, diags
	}
	return notificationOrderFromAPI(o), diags
}

func notificationOrderFromAPI(o *client.NotificationOrder) notificationOrderModel {
	m := notificationOrderModel{ID: types.StringValue(notificationOrderID), Enabled: types.BoolValue(o.Enabled)}
	for _, t := range o.Tiers {
		m.Tiers = append(m.Tiers, notificationTier{Channels: t.Channels, DelaySeconds: types.Int64Value(t.DelaySeconds)})
	}
	return m
}
