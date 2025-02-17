// Package provider is a internal package containing the Katapult Terraform
// provider.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
	corenext "github.com/krystal/go-katapult/next/core"
)

const defaultGeneratedNamePrefix = "tf"

var once sync.Once

type Config struct {
	Version    string
	Commit     string
	HTTPClient *http.Client

	GeneratedNamePrefix string
}

func New(c *Config) func() *schema.Provider { //nolint:funlen
	once.Do(func() {
		// Set descriptions to support markdown syntax, this will be used in
		// document generation and the language server.
		schema.DescriptionKind = schema.StringMarkdown

		// Customize the content of descriptions when output. For example you
		// can add defaults on to the exported descriptions if present.
		schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
			desc := s.Description
			if s.Default != nil {
				desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
			}

			return strings.TrimSpace(desc)
		}
	})

	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"api_key": {
					Type:      schema.TypeString,
					Optional:  true,
					Sensitive: true,
					Description: "**REQUIRED** via config or " +
						"environment variable. " +
						"API Key for Katapult Core API. Can be " +
						"specified with the `KATAPULT_API_KEY` environment " +
						"variable.",
				},
				"organization": {
					Type:     schema.TypeString,
					Optional: true,

					Description: "**REQUIRED** via config or " +
						"environment variable. " +
						"Organization sub-domain. Can be " +
						"specified with the `KATAPULT_ORGANIZATION` " +
						"environment variable.",
				},
				"data_center": {
					Type:     schema.TypeString,
					Optional: true,

					Description: "**REQUIRED** via config or " +
						"environment variable. " +
						"Data center permalink. Can be " +
						"specified with the `KATAPULT_DATA_CENTER` " +
						"environment variable.",
				},
				"skip_trash_object_purge": {
					Type:     schema.TypeBool,
					Optional: true,
					//nolint:lll
					Description: strings.TrimSpace(`

Skip purging deleted resources from Katapult's trash when they are destroyed by Terraform. Only relevant to some resources which are moved to the trash when they are deleted. Can be specified with the
` + "`KATAPULT_SKIP_TRASH_OBJECT_PURGE`" + ` environment variable. Defaults to ` + "`false`" + `.

  ~> **Note:** Using ` + "`skip_trash_object_purge`" + ` can quickly lead to a build up of a lot objects in the trash if you are replacing resources repeatedly. Hence this option is disabled by default, and should only be used if you are sure you want to keep deleted resources in the trash.

`),
				},
				"log_level": {
					Type:     schema.TypeString,
					Optional: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_LOG_LEVEL", "info",
					),
					ValidateFunc: validation.StringInSlice(
						[]string{
							"trace",
							"debug",
							"info",
							"warn",
							"error",
							"off",
						}, true,
					),
					Description: "Log level used by Katapult Terraform " +
						"provider. Can be specified with the " +
						"`KATAPULT_LOG_LEVEL` environment variable. " +
						"Defaults to `info`.",
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"katapult_security_group":        resourceSecurityGroup(),
				"katapult_security_group_rule":   resourceSecurityGroupRule(),
				"katapult_virtual_machine":       resourceVirtualMachine(),
				"katapult_virtual_machine_group": resourceVirtualMachineGroup(),
			},
			//nolint:lll
			DataSourcesMap: map[string]*schema.Resource{
				"katapult_data_center":              dataSourceDataCenter(),
				"katapult_disk_template":            dataSourceDiskTemplate(),
				"katapult_disk_templates":           dataSourceDiskTemplates(),
				"katapult_network_speed_profile":    dataSourceNetworkSpeedProfile(),
				"katapult_network_speed_profiles":   dataSourceNetworkSpeedProfiles(),
				"katapult_security_group":           dataSourceSecurityGroup(),
				"katapult_security_group_rule":      dataSourceSecurityGroupRule(),
				"katapult_security_group_rules":     dataSourceSecurityGroupRules(),
				"katapult_security_groups":          dataSourceSecurityGroups(),
				"katapult_virtual_machine":          dataSourceVirtualMachine(),
				"katapult_virtual_machine_group":    dataSourceVirtualMachineGroup(),
				"katapult_virtual_machine_groups":   dataSourceVirtualMachineGroups(),
				"katapult_virtual_machine_package":  dataSourceVirtualMachinePackage(),
				"katapult_virtual_machine_packages": dataSourceVirtualMachinePackages(),
			},
		}

		if os.Getenv("TF_ACC") == "1" {
			// TEST RESOURCES
			p.ResourcesMap["katapult_legacy_ip"] = resourceIP()

			//nolint:lll // This is a test resource.
			p.ResourcesMap["katapult_legacy_file_storage_volume"] = resourceFileStorageVolume()

			// TEST DATA SOURCES

			//nolint:lll // This is a test resource.
			p.DataSourcesMap["katapult_legacy_file_storage_volume"] = dataSourceFileStorageVolume()

			//nolint:lll // This is a test resource.
			p.DataSourcesMap["katapult_legacy_file_storage_volumes"] = dataSourceFileStorageVolumes()
		}

		p.ConfigureContextFunc = configure(c, p)

		return p
	}
}

func stringOrEnv(in string, env string) string {
	if in != "" {
		return in
	}

	return os.Getenv(env)
}

func boolOrEnv(in bool, env string) bool {
	if in {
		return true
	}

	switch strings.ToLower(os.Getenv(env)) {
	case "true", "1", "yes", "on", "y", "t":
		return true
	}

	return false
}

//nolint:funlen
func configure(
	conf *Config,
	p *schema.Provider,
) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(
		_ context.Context,
		d *schema.ResourceData,
	) (interface{}, diag.Diagnostics) {
		logLevel := stringOrEnv(
			d.Get("log_level").(string),
			"KATAPULT_LOG_LEVEL",
		)
		if logLevel == "" {
			logLevel = "info"
		}

		m := &Meta{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:       "katapult",
				Level:      hclog.LevelFromString(logLevel),
				TimeFormat: "2006/01/02 15:04:05",
			}),
			confAPIKey: stringOrEnv(
				d.Get("api_key").(string),
				"KATAPULT_API_KEY",
			),
			confDataCenter: stringOrEnv(
				d.Get("data_center").(string),
				"KATAPULT_DATA_CENTER",
			),
			confOrganization: stringOrEnv(
				d.Get("organization").(string),
				"KATAPULT_ORGANIZATION",
			),
			SkipTrashObjectPurge: boolOrEnv(
				d.Get("skip_trash_object_purge").(bool),
				"KATAPULT_SKIP_TRASH_OBJECT_PURGE",
			),
			GeneratedNamePrefix: conf.GeneratedNamePrefix,
		}

		if m.GeneratedNamePrefix == "" {
			m.GeneratedNamePrefix = defaultGeneratedNamePrefix
		}

		opts := []katapult.Option{
			katapult.WithAPIKey(m.confAPIKey),
			katapult.WithUserAgent(
				p.UserAgent("terraform-provider-katapult", conf.Version),
			),
		}

		httpClient := conf.HTTPClient
		if httpClient == nil {
			httpClient = &http.Client{Timeout: 60 * time.Second}
		}

		if conf.HTTPClient != nil {
			opts = append(opts, katapult.WithHTTPClient(httpClient))
		}

		// Debug override of API URL for internal testing purposes.
		if apiURL := os.Getenv("KATAPULT_TF_DEBUG_API_URL"); apiURL != "" {
			u, err := url.Parse(apiURL)
			if err != nil {
				return m, diag.FromErr(err)
			}

			opts = append(opts, katapult.WithBaseURL(u))
		}

		c, err := katapult.New(opts...)
		if err != nil {
			return m, diag.FromErr(err)
		}

		rhc := newRetryableHTTPClient(httpClient, m.Logger)
		c.HTTPClient = rhc.StandardClient()

		m.Client = c
		m.Core = core.New(m.Client)

		// Initialize CoreNext client following v6provider approach
		baseURL := m.Client.BaseURL
		if baseURL.Path == "" || baseURL.Path == "/" {
			baseURL.Path = "/core/v1"
		}

		coreNextClient, err := corenext.NewClientWithResponses(
			baseURL.String(),
			m.confAPIKey,
			corenext.WithHTTPClient(rhc.StandardClient()),
			corenext.WithRequestEditorFn(
				func(_ context.Context, req *http.Request) error {
					req.Header.Set(
						"User-Agent",
						p.UserAgent(
							"terraform-provider-katapult", conf.Version,
						),
					)
					return nil
				},
			),
		)
		if err != nil {
			return m, diag.FromErr(err)
		}

		m.CoreNext = coreNextClient

		m.OrganizationRef = core.OrganizationRef{
			SubDomain: m.confOrganization,
		}
		m.DataCenterRef = core.DataCenterRef{
			Permalink: m.confDataCenter,
		}

		return m, nil
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
	client.RetryMax = 4
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
