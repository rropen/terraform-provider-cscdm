// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-csc-domain-manager/internal/util"
)

const (
	CSC_DOMAIN_MANAGER_API_URL = "https://apis.cscglobal.com/dbs/api/v2/"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &CscDomainManagerProvider{}
)

// CscDomainManagerProvider is the provider implementation.
type CscDomainManagerProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ScaffoldingProviderModel describes the provider data model.
type CscDomainManagerProviderModel struct {
	ApiKey   types.String `tfsdk:"api_key"`
	ApiToken types.String `tfsdk:"api_token"`
}

// Metadata returns the provider type name.
func (p *CscDomainManagerProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cscdm"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *CscDomainManagerProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "CSC Domain Manager API Key",
				Optional:    true,
				Sensitive:   true,
			},
			"api_token": schema.StringAttribute{
				Description: "CSC Domain Manager API Token",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// Configure prepares a HashiCups API client for data sources and resources.
func (p *CscDomainManagerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring CSC Domain Manager client")

	var config CscDomainManagerProviderModel
	diags := req.Config.Get(ctx, &config)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ApiKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown CSC Domain Manager API Key",
			"The provider cannot create the CSC Domain Manager API client as there is an unknown configuration value for the API key. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CSCDM_API_KEY environment variable.",
		)
	}

	if config.ApiToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Unknown CSC Domain Manager API Token",
			"The provider cannot create the CSC Domain Manager API client as there is an unknown configuration value for the API token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CSCDM_API_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	api_key := os.Getenv("CSCDM_API_KEY")
	api_token := os.Getenv("CSCDM_API_TOKEN")

	if !config.ApiKey.IsNull() {
		api_key = config.ApiKey.ValueString()
	}

	if !config.ApiToken.IsNull() {
		api_token = config.ApiToken.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.
	if api_key == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing CSC Domain Manager API Key",
			"The provider cannot create the CSC Domain Manager API client as there is a missing or empty value for the API key. "+
				"Set the host value in the configuration or use the CSCDM_API_KEY environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if api_token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing CSC Domain Manager API Token",
			"The provider cannot create the CSC Domain Manager API client as there is a missing or empty value for the API token. "+
				"Set the host value in the configuration or use the CSCDM_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "cscdm_api_key", api_key)
	ctx = tflog.SetField(ctx, "cscdm_api_token", api_token)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "cscdm_api_key", "cscdm_api_token")

	// Make HTTP client available during DataSource and Resource Configure methods.
	client := &http.Client{Transport: &util.HttpTransport{
		BaseUrl: CSC_DOMAIN_MANAGER_API_URL,
		Headers: map[string]string{
			"accept":        "application/json",
			"apikey":        api_key,
			"Authorization": fmt.Sprintf("Bearer %s", api_token),
		},
	}}
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "Configured CSC Domain Manager client")
}

// DataSources defines the data sources implemented in the provider.
func (p *CscDomainManagerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewZonesDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *CscDomainManagerProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CscDomainManagerProvider{
			version: version,
		}
	}
}
