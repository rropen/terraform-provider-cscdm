package provider

import (
	"context"
	"fmt"
	"strings"
	"terraform-provider-cscdm/internal/cscdm"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &RecordResource{}
	_ resource.ResourceWithConfigure   = &RecordResource{}
	_ resource.ResourceWithImportState = &RecordResource{}
)

// NewRecordResource is a helper function to simplify the provider implementation.
func NewRecordResource() resource.Resource {
	return &RecordResource{}
}

// RecordResource is the resource implementation.
type RecordResource struct {
	client *cscdm.Client
}

type RecordResourceModel struct {
	Zone        types.String `tfsdk:"zone"`
	Type        types.String `tfsdk:"type"`
	Id          types.String `tfsdk:"id"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Ttl         types.Int64  `tfsdk:"ttl"`
	Priority    types.Int64  `tfsdk:"priority"`
	Status      types.String `tfsdk:"status"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

// Metadata returns the resource type name.
func (r *RecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_record"
}

// Schema defines the schema for the resource.
func (r *RecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("A", "AAAA", "CNAME", "MX", "NS", "TXT"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
			"key": schema.StringAttribute{
				Required: true,
			},
			"value": schema.StringAttribute{
				Required: true,
			},
			"ttl": schema.Int64Attribute{
				Optional: true,
			},
			"priority": schema.Int64Attribute{
				Optional: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *RecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Add a nil check when handling ProviderData because Terraform
	// sets that data after it calls the ConfigureProvider RPC.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*cscdm.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *cscdm.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func copyRecord(dst *RecordResourceModel, src *cscdm.ZoneRecord) {
	dst.Id = types.StringValue(src.Id)
	dst.Key = types.StringValue(src.Key)
	dst.Value = types.StringValue(src.Value)

	if src.Ttl == 0 {
		dst.Ttl = types.Int64Null()
	} else {
		dst.Ttl = types.Int64Value(src.Ttl)
	}

	if src.Priority == 0 {
		dst.Priority = types.Int64Null()
	} else {
		dst.Priority = types.Int64Value(src.Priority)
	}

	dst.Status = types.StringValue(src.Status)
}

// Create creates the resource and sets the initial Terraform state.
func (r *RecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan RecordResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	recordAction := cscdm.RecordAction{
		ZoneEdit: cscdm.ZoneEdit{
			Action:      "ADD",
			RecordType:  plan.Type.ValueString(),
			NewKey:      plan.Key.ValueString(),
			NewValue:    plan.Value.ValueString(),
			NewTtl:      plan.Ttl.ValueInt64(),
			NewPriority: plan.Priority.ValueInt64(),
		},
		ZoneName: plan.Zone.ValueString(),
	}

	zoneRecord, err := r.client.PerformRecordAction(&recordAction)
	if err != nil {
		resp.Diagnostics.AddError("error creating record", err.Error())
		return
	}

	copyRecord(&plan, zoneRecord)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *RecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state RecordResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.client.GetZone(state.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error fetching zone", err.Error())
		return
	}

	record, err := r.client.GetRecordByTypeById(zone, state.Type.ValueString(), state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error getting record from zone", err.Error())
		return
	}

	copyRecord(&state, record)

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *RecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan RecordResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Retrieve current state
	var state RecordResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	recordAction := cscdm.RecordAction{
		ZoneEdit: cscdm.ZoneEdit{
			Action:       "EDIT",
			RecordType:   state.Type.ValueString(),
			CurrentKey:   state.Key.ValueString(),
			CurrentValue: state.Value.ValueString(),
			NewKey:       plan.Key.ValueString(),
			NewValue:     plan.Value.ValueString(),
			NewTtl:       plan.Ttl.ValueInt64(),
			NewPriority:  plan.Priority.ValueInt64(),
		},
		ZoneName: plan.Zone.ValueString(),
	}

	zoneRecord, err := r.client.PerformRecordAction(&recordAction)
	if err != nil {
		resp.Diagnostics.AddError("error updating record", err.Error())
		return
	}

	copyRecord(&plan, zoneRecord)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *RecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve current state
	var state RecordResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	recordAction := cscdm.RecordAction{
		ZoneEdit: cscdm.ZoneEdit{
			Action:       "PURGE",
			RecordType:   state.Type.ValueString(),
			CurrentKey:   state.Key.ValueString(),
			CurrentValue: state.Value.ValueString(),
		},
		ZoneName: state.Zone.ValueString(),
	}

	_, err := r.client.PerformRecordAction(&recordAction)
	if err != nil {
		resp.Diagnostics.AddError("error updating record", err.Error())
		return
	}
}

func (r *RecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ":")

	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"unexpected import identifier",
			fmt.Sprintf("expected import identifier with format: `zone:type:id`, got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone_name"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), idParts[2])...)
}
