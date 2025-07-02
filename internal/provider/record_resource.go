package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &RecordResource{}
	_ resource.ResourceWithConfigure = &RecordResource{}
)

// NewRecordResource is a helper function to simplify the provider implementation.
func NewRecordResource() resource.Resource {
	return &RecordResource{}
}

// RecordResource is the resource implementation.
type RecordResource struct {
	client *http.Client
}

type RecordResourceModel struct {
	ZoneName    types.String `tfsdk:"zone_name"`
	Type        types.String `tfsdk:"type"`
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
			"zone_name": schema.StringAttribute{
				Required: true,
			},
			"type": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("A", "AAAA", "CNAME", "MX", "NS", "TXT"),
				},
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

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

type RecordCreateJson struct {
	ZoneName string       `json:"zoneName"`
	Edits    []RecordEdit `json:"edits"`
}

type RecordEdit struct {
	RecordType  string `json:"recordType"`
	Action      string `json:"action"`
	NewKey      string `json:"newKey"`
	NewValue    string `json:"newValue"`
	NewTtl      int64  `json:"newTtl,omitempty"`
	NewPriority int64  `json:"newPriority,omitempty"`
}

type RecordCreateRespJson struct {
	Content struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"content"`
	Links struct {
		Self   string `json:"self"`
		Status string `json:"status"`
	} `json:"links"`
}

type RecordCreateErrJson struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Value       string `json:"value"`
}

type EditStatusJson struct {
	Content struct {
		Status string `json:"status"`
	} `json:"content"`
}

func createRecord(client *http.Client, payload RecordCreateJson) (*string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal record payload: %s", err)
	}

	for {
		createResp, err := client.Post("zones/edits", "application/json", bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %s", err)
		}

		if createResp.StatusCode != 201 {
			var createErrJson RecordCreateErrJson
			err = json.NewDecoder(createResp.Body).Decode(&createErrJson)
			if err != nil {
				return nil, fmt.Errorf("unable to unmarshal create record error response: %s", err)
			}

			if createErrJson.Code == "OPEN_ZONE_EDITS" {
				time.Sleep(5 * time.Second)
				continue
			}

			return nil, fmt.Errorf("request returned unsuccessful status code: %s", err)
		}

		var createJson RecordCreateRespJson
		err = json.NewDecoder(createResp.Body).Decode(&createJson)
		if err != nil {
			return nil, fmt.Errorf("unable to unmarshal create record response: %s", err)
		}

		editStatusLink := strings.Split(createJson.Links.Status, "/")
		return &editStatusLink[len(editStatusLink)-1], nil
	}
}

func waitForRecordEdit(client *http.Client, editId string) error {
	for {
		editStatusResp, err := client.Get(fmt.Sprintf("zones/edits/status/%s", editId))
		if err != nil {
			return fmt.Errorf("failed to send request: %s", err)
		}

		var editStatusJson EditStatusJson
		err = json.NewDecoder(editStatusResp.Body).Decode(&editStatusJson)
		if err != nil {
			return fmt.Errorf("unable to unmarshal edit status response: %s", err)
		}

		if editStatusJson.Content.Status == "COMPLETED" {
			return nil
		}

		time.Sleep(5 * time.Second)
	}
}

func getZone(client *http.Client, zoneName string) (*ZoneJson, error) {
	zoneResp, err := client.Get(fmt.Sprintf("zones/%s", zoneName))
	if err != nil {
		return nil, fmt.Errorf("unable to send request: %s", err)
	}

	var zoneJson ZoneJson
	err = json.NewDecoder(zoneResp.Body).Decode(&zoneJson)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal zone: %s", err)
	}

	return &zoneJson, nil
}

func filterRecordsByType(zone *ZoneJson, recordType string) []ZoneRecordJson {
	switch recordType {
	case "A":
		return zone.A
	case "AAAA":
		return zone.AAAA
	case "CNAME":
		return zone.CNAME
	case "MX":
		return zone.MX
	case "NS":
		return zone.NS
	case "TXT":
		return zone.TXT
	default:
		return nil
	}
}

func findRecordByKey(records []ZoneRecordJson, key string) *ZoneRecordJson {
	for _, record := range records {
		if record.Key == key {
			return &record
		}
	}

	return nil
}

func setRecord(dst *RecordResourceModel, src *ZoneRecordJson) {
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

func setRecordState(state *RecordResourceModel, client *http.Client) error {
	zone, err := getZone(client, state.ZoneName.ValueString())
	if err != nil {
		return fmt.Errorf("error reading zone: %s", err.Error())
	}

	records := filterRecordsByType(zone, state.Type.ValueString())
	if records == nil {
		return fmt.Errorf("unsupported record type: %s", state.Type.ValueString())
	}

	record := findRecordByKey(records, state.Key.ValueString())
	if record == nil {
		return fmt.Errorf("record of type %s with key '%s' was not found in zone %s", state.Type.ValueString(), state.Key.ValueString(), state.ZoneName.ValueString())
	}

	setRecord(state, record)
	return nil
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

	payload := RecordCreateJson{
		ZoneName: plan.ZoneName.ValueString(),
		Edits: []RecordEdit{
			{
				RecordType:  plan.Type.ValueString(),
				Action:      "ADD",
				NewKey:      plan.Key.ValueString(),
				NewValue:    plan.Value.ValueString(),
				NewTtl:      plan.Ttl.ValueInt64(),
				NewPriority: plan.Priority.ValueInt64(),
			},
		},
	}

	editId, err := createRecord(r.client, payload)
	if err != nil {
		resp.Diagnostics.AddError("error creating record", err.Error())
		return
	}

	err = waitForRecordEdit(r.client, *editId)
	if err != nil {
		resp.Diagnostics.AddError("error waiting for record creation", err.Error())
		return
	}

	err = setRecordState(&plan, r.client)
	if err != nil {
		resp.Diagnostics.AddError("error setting record state", err.Error())
		return
	}

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

	err := setRecordState(&state, r.client)
	if err != nil {
		resp.Diagnostics.AddError("error setting record state", err.Error())
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *RecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *RecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}
