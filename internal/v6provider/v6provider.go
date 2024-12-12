package v6provider

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultGeneratedNamePrefix = "tf"
	uaEnvVar                   = "TF_APPEND_USER_AGENT"
)

type (
	KatapultProvider struct {
		Version    string
		Commit     string
		HTTPClient *http.Client

		GeneratedNamePrefix string
		m                   *Meta
	}

	KatapultProviderModel struct {
		APIKey               types.String `tfsdk:"api_key"`
		Organization         types.String `tfsdk:"organization"`
		DataCenter           types.String `tfsdk:"data_center"`
		SkipTrashObjectPurge types.Bool   `tfsdk:"skip_trash_object_purge"`
		LogLevel             types.String `tfsdk:"log_level"`
	}
)

func New(k *KatapultProvider) func() provider.Provider {
	return func() provider.Provider {
		if k != nil {
			return k
		}

		return &KatapultProvider{}
	}
}

func (k *KatapultProvider) Metadata(
	_ context.Context,
	_ provider.MetadataRequest,
	resp *provider.MetadataResponse,
) {
	resp.TypeName = "katapult"
}

func (k *KatapultProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "**REQUIRED** via config or " +
					"environment variable. " +
					"API Key for Katapult Core API. Can be " +
					"specified with the `KATAPULT_API_KEY` environment " +
					"variable.",
			},
			"organization": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "**REQUIRED** via config or " +
					"environment variable. " +
					"Organization sub-domain. Can be " +
					"specified with the `KATAPULT_ORGANIZATION` " +
					"environment variable.",
			},
			"data_center": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "**REQUIRED** via config or " +
					"environment variable. " +
					"Data center permalink. Can be " +
					"specified with the `KATAPULT_DATA_CENTER` " +
					"environment variable.",
			},
			"skip_trash_object_purge": schema.BoolAttribute{
				Optional: true,
				//nolint:lll
				Description: strings.TrimSpace(`

Skip purging deleted resources from Katapult's trash when they are destroyed by Terraform. Only relevant to some resources which are moved to the trash when they are deleted. Can be specified with the
` + "`KATAPULT_SKIP_TRASH_OBJECT_PURGE`" + ` environment variable. Defaults to ` + "`false`" + `.

  ~> **Note:** Using ` + "`skip_trash_object_purge`" + ` can quickly lead to a build up of a lot objects in the trash if you are replacing resources repeatedly. Hence this option is disabled by default, and should only be used if you are sure you want to keep deleted resources in the trash.

`),
				//nolint:lll
				MarkdownDescription: strings.TrimSpace(`

Skip purging deleted resources from Katapult's trash when they are destroyed by Terraform. Only relevant to some resources which are moved to the trash when they are deleted. Can be specified with the
` + "`KATAPULT_SKIP_TRASH_OBJECT_PURGE`" + ` environment variable. Defaults to ` + "`false`" + `.

  ~> **Note:** Using ` + "`skip_trash_object_purge`" + ` can quickly lead to a build up of a lot objects in the trash if you are replacing resources repeatedly. Hence this option is disabled by default, and should only be used if you are sure you want to keep deleted resources in the trash.

`),
			},
			"log_level": schema.StringAttribute{
				Optional: true,

				Validators: []validator.String{
					stringvalidator.OneOfCaseInsensitive("trace",
						"debug",
						"info",
						"warn",
						"error",
						"off",
					),
				},
				Description: "Log level used by Katapult Terraform " +
					"provider. Can be specified with the " +
					"`KATAPULT_LOG_LEVEL` environment variable. " +
					"Defaults to `info`.",
				MarkdownDescription: "Log level used by Katapult Terraform " +
					"provider. Can be specified with the " +
					"`KATAPULT_LOG_LEVEL` environment variable. " +
					"Defaults to `info`.",
			},
		},
	}
}

func stringOrEnv(in string, env string) string {
	if in != "" {
		return in
	}

	return os.Getenv(env)
}

func boolOrEnv(in *bool, env string) bool {
	if in != nil {
		return *in
	}

	switch strings.ToLower(os.Getenv(env)) {
	case "true", "1", "yes", "on", "y", "t":
		return true
	}

	return false
}

// getTagValue returns the value of a tag for a field in a struct. Primarily
// used for getting the value of the `tfsdk` tag in custom plan modifiers.
func getTagValue(st interface{}, field string, tag string) string {
	rType := reflect.TypeOf(st)
	fieldType, ok := rType.FieldByName(field)
	if !ok {
		return ""
	}

	return fieldType.Tag.Get(tag)
}

func userAgent(name string, terraformVersion string, version string) string {
	ua := fmt.Sprintf(
		"Terraform/%s (+https://www.terraform.io) Terraform-Plugin-Framework",
		terraformVersion,
	)
	if name != "" {
		ua += " " + name
		if version != "" {
			ua += "/" + version
		}
	}

	if add := os.Getenv(uaEnvVar); add != "" {
		add = strings.TrimSpace(add)
		if len(add) > 0 {
			ua += " " + add
			log.Printf("[DEBUG] Using modified User-Agent: %s", ua)
		}
	}

	return ua
}

func (k *KatapultProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
	if k.m != nil {
		resp.ResourceData = k.m
		resp.DataSourceData = k.m
		return
	}

	var conf KatapultProviderModel
	diags := req.Config.Get(ctx, &conf)
	resp.Diagnostics.Append(diags...)

	m, err := NewMeta(
		conf.APIKey.ValueString(),
		conf.DataCenter.ValueString(),
		conf.Organization.ValueString(),
		conf.SkipTrashObjectPurge.ValueBoolPointer(),
		conf.LogLevel.ValueString(),
		k.GeneratedNamePrefix,
		k.HTTPClient,
		k.Version,
		req.TerraformVersion,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Configure Error",
			err.Error(),
		)
		return
	}

	k.m = m
	resp.ResourceData = m
	resp.DataSourceData = m
}

func (k *KatapultProvider) Resources(
	_ context.Context,
) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &AddressListEntryResource{} },
		func() resource.Resource { return &AddressListResource{} },
		func() resource.Resource { return &IPResource{} },
		func() resource.Resource { return &LoadBalancerResource{} },
		func() resource.Resource { return &LoadBalancerRuleResource{} },
		func() resource.Resource { return &VirtualNetworkResource{} },
	}
}

func (k *KatapultProvider) DataSources(
	_ context.Context,
) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &AddressListDataSource{} },
		func() datasource.DataSource { return &AddressListEntriesDataSource{} },
		func() datasource.DataSource { return &AddressListEntryDataSource{} },
		func() datasource.DataSource { return &AddressListsDataSource{} },
		func() datasource.DataSource { return &GlobalAddressListsDataSource{} },
		func() datasource.DataSource { return &IPDataSource{} },
		func() datasource.DataSource { return &LoadBalancerDataSource{} },
		func() datasource.DataSource { return &LoadBalancerRuleDataSource{} },
		func() datasource.DataSource { return &LoadBalancerRulesDataSource{} },
		func() datasource.DataSource { return &LoadBalancersDataSource{} },
		func() datasource.DataSource { return &NetworkDataSource{} },
		func() datasource.DataSource { return &NetworksDataSource{} },
		func() datasource.DataSource { return &VirtualNetworkDataSource{} },
	}
}

func newRetryableHTTPClient(
	httpClient *http.Client,
	logger hclog.Logger,
) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.HTTPClient = httpClient
	client.Logger = logger

	client.RetryWaitMin = 1 * time.Second
	client.RetryWaitMax = 2 * time.Minute
	client.RetryMax = 10
	client.CheckRetry = requestRetryPolicy

	return client
}

func requestRetryPolicy(
	ctx context.Context,
	resp *http.Response,
	err error,
) (bool, error) {
	if resp == nil || resp.StatusCode == http.StatusTooManyRequests {
		return true, err
	}

	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}
