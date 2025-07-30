package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ datasource.DataSource              = &ZonesDataSource{}
	_ datasource.DataSourceWithConfigure = &ZonesDataSource{}
)

func NewZonesDataSource() datasource.DataSource {
	return &ZonesDataSource{}
}

// ZonesDataSource defines the data source implementation.
type ZonesDataSource struct {
	client *http.Client
}

type ZonesDataSourceModel struct {
	Zones []ZoneModel  `tfsdk:"zones"`
	Name  types.String `tfsdk:"name"`
}

type ZoneModel struct {
	ZoneName    types.String         `tfsdk:"zone_name"`
	HostingType types.String         `tfsdk:"hosting_type"`
	A           []ZoneRecordModel    `tfsdk:"a"`
	AAAA        []ZoneRecordModel    `tfsdk:"aaaa"`
	CNAME       []ZoneRecordModel    `tfsdk:"cname"`
	MX          []ZoneRecordModel    `tfsdk:"mx"`
	NS          []ZoneRecordModel    `tfsdk:"ns"`
	TXT         []ZoneRecordModel    `tfsdk:"txt"`
	SRV         []ZoneSrvRecordModel `tfsdk:"srv"`
	CAA         []ZoneRecordModel    `tfsdk:"caa"`
	SOA         ZoneSoaRecordModel   `tfsdk:"soa"`
}

type ZoneRecordModel struct {
	Id       types.String `tfsdk:"id"`
	Key      types.String `tfsdk:"key"`
	Value    types.String `tfsdk:"value"`
	Ttl      types.Int64  `tfsdk:"ttl"`
	Status   types.String `tfsdk:"status"`
	Priority types.Int64  `tfsdk:"priority"`
}

type ZoneSrvRecordModel struct {
	ZoneRecordModel
	Port types.Int32 `tfsdk:"port"`
}

type ZoneSoaRecordModel struct {
	Serial     types.Int64  `tfsdk:"serial"`
	Refresh    types.Int64  `tfsdk:"refresh"`
	Retry      types.Int64  `tfsdk:"retry"`
	Expire     types.Int64  `tfsdk:"expire"`
	TtlMin     types.Int64  `tfsdk:"ttl_min"`
	TtlNeg     types.Int64  `tfsdk:"ttl_neg"`
	TtlZone    types.Int64  `tfsdk:"ttl_zone"`
	TechEmail  types.String `tfsdk:"tech_email"`
	MasterHost types.String `tfsdk:"master_host"`
}

func (d *ZonesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zones"
}

func (d *ZonesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	RecordListAttrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true,
		},
		"key": schema.StringAttribute{
			Computed: true,
		},
		"value": schema.StringAttribute{
			Computed: true,
		},
		"ttl": schema.Int64Attribute{
			Computed: true,
		},
		"status": schema.StringAttribute{
			Computed: true,
		},
		"priority": schema.Int64Attribute{
			Computed: true,
		},
	}
	RecordList := schema.ListNestedAttribute{
		Computed: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: RecordListAttrs,
		},
	}

	SrvRecordListAttrs := make(map[string]schema.Attribute)
	for k, v := range RecordListAttrs {
		SrvRecordListAttrs[k] = v
	}
	SrvRecordListAttrs["port"] = schema.Int32Attribute{
		Computed: true,
	}
	SrvRecordList := schema.ListNestedAttribute{
		Computed: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: SrvRecordListAttrs,
		},
	}

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"zones": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"zone_name": schema.StringAttribute{
							Computed: true,
						},
						"hosting_type": schema.StringAttribute{
							Computed: true,
						},
						"a":     RecordList,
						"aaaa":  RecordList,
						"cname": RecordList,
						"mx":    RecordList,
						"ns":    RecordList,
						"txt":   RecordList,
						"srv":   SrvRecordList,
						"caa":   RecordList,
						"soa": schema.SingleNestedAttribute{
							Computed: true,
							Attributes: map[string]schema.Attribute{
								"serial": schema.Int64Attribute{
									Computed: true,
								},
								"refresh": schema.Int64Attribute{
									Computed: true,
								},
								"retry": schema.Int64Attribute{
									Computed: true,
								},
								"expire": schema.Int64Attribute{
									Computed: true,
								},
								"ttl_min": schema.Int64Attribute{
									Computed: true,
								},
								"ttl_neg": schema.Int64Attribute{
									Computed: true,
								},
								"ttl_zone": schema.Int64Attribute{
									Computed: true,
								},
								"tech_email": schema.StringAttribute{
									Computed: true,
								},
								"master_host": schema.StringAttribute{
									Computed: true,
								},
							},
						},
					},
				},
			},
			"name": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}

func (d *ZonesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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

	d.client = client
}

type ZonesJson struct {
	Meta struct {
		NumResults int64 `json:"numResults"`
		Pages      int64 `json:"pages"`
	} `json:"meta"`
	Zones []ZoneJson `json:"zones"`
	Links struct {
		Self string `json:"self"`
	} `json:"links"`
}

type ZoneJson struct {
	ZoneName    string              `json:"zoneName"`
	HostingType string              `json:"hostingType"`
	A           []ZoneRecordJson    `json:"a"`
	CNAME       []ZoneRecordJson    `json:"cname"`
	AAAA        []ZoneRecordJson    `json:"aaaa"`
	TXT         []ZoneRecordJson    `json:"txt"`
	MX          []ZoneRecordJson    `json:"mx"`
	NS          []ZoneRecordJson    `json:"ns"`
	SRV         []ZoneSrvRecordJson `json:"srv"`
	CAA         []ZoneRecordJson    `json:"caa"`
	SOA         ZoneSoaRecordJson   `json:"soa"`
}

type ZoneRecordJson struct {
	Id       string `json:"id"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Ttl      int64  `json:"ttl,omitempty"`
	Status   string `json:"status"`
	Priority int64  `json:"priority"`
}

type ZoneSrvRecordJson struct {
	ZoneRecordJson
	Port int32 `json:"port"`
}

type ZoneSoaRecordJson struct {
	Serial     int64  `json:"serial"`
	Refresh    int64  `json:"refresh"`
	Retry      int64  `json:"retry"`
	Expire     int64  `json:"expire"`
	TtlMin     int64  `json:"ttlMin"`
	TtlNeg     int64  `json:"ttlNeg"`
	TtlZone    int64  `json:"ttlZone"`
	TechEmail  string `json:"techEmail"`
	MasterHost string `json:"masterHost"`
}

func convertZone(zone ZoneJson) ZoneModel {
	return ZoneModel{
		ZoneName:    types.StringValue(zone.ZoneName),
		HostingType: types.StringValue(zone.HostingType),
		A:           convertZoneRecords(zone.A),
		AAAA:        convertZoneRecords(zone.AAAA),
		CNAME:       convertZoneRecords(zone.CNAME),
		MX:          convertZoneRecords(zone.MX),
		NS:          convertZoneRecords(zone.NS),
		TXT:         convertZoneRecords(zone.TXT),
		SRV:         convertZoneSrvRecords(zone.SRV),
		CAA:         convertZoneRecords(zone.CAA),
		SOA:         convertZoneSoaRecord(zone.SOA),
	}
}

func convertZoneRecord(rec ZoneRecordJson) ZoneRecordModel {
	return ZoneRecordModel{
		Id:       types.StringValue(rec.Id),
		Key:      types.StringValue(rec.Key),
		Value:    types.StringValue(rec.Value),
		Ttl:      types.Int64Value(rec.Ttl),
		Status:   types.StringValue(rec.Status),
		Priority: types.Int64Value(rec.Priority),
	}
}

func convertZoneRecords(recs []ZoneRecordJson) []ZoneRecordModel {
	records := make([]ZoneRecordModel, len(recs))

	for i, rec := range recs {
		records[i] = convertZoneRecord(rec)
	}

	return records
}

func convertZoneSrvRecords(recs []ZoneSrvRecordJson) []ZoneSrvRecordModel {
	records := make([]ZoneSrvRecordModel, len(recs))

	for i, rec := range recs {
		records[i] = ZoneSrvRecordModel{
			ZoneRecordModel: convertZoneRecord(rec.ZoneRecordJson),
			Port:            types.Int32Value(rec.Port),
		}
	}

	return records
}

func convertZoneSoaRecord(rec ZoneSoaRecordJson) ZoneSoaRecordModel {
	return ZoneSoaRecordModel{
		Serial:     types.Int64Value(rec.Serial),
		Refresh:    types.Int64Value(rec.Refresh),
		Retry:      types.Int64Value(rec.Retry),
		Expire:     types.Int64Value(rec.Expire),
		TtlMin:     types.Int64Value(rec.TtlMin),
		TtlNeg:     types.Int64Value(rec.TtlNeg),
		TtlZone:    types.Int64Value(rec.TtlZone),
		TechEmail:  types.StringValue(rec.TechEmail),
		MasterHost: types.StringValue(rec.MasterHost),
	}
}

func (d *ZonesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ZonesDataSourceModel
	var diags diag.Diagnostics

	diags = resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Name != types.StringNull() {
		var zoneJson ZoneJson
		zonesResp, err := d.client.Get(fmt.Sprintf("zones/%s", state.Name.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read desired zone, got error: %s", err))
			return
		}
		defer zonesResp.Body.Close()
		err = json.NewDecoder(zonesResp.Body).Decode(&zoneJson)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unmarshal desired zone, got error: %s", err))
			return
		}
		state.Zones = append(state.Zones, convertZone(zoneJson))
	} else {
		var zonesJson ZonesJson
		zonesResp, err := d.client.Get("zones")
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read zones, got error: %s", err))
			return
		}
		defer zonesResp.Body.Close()
		err = json.NewDecoder(zonesResp.Body).Decode(&zonesJson)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unmarshal zones, got error: %s", err))
			return
		}
		for _, zone := range zonesJson.Zones {
			state.Zones = append(state.Zones, convertZone(zone))
		}
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
