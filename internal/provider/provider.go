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
)

const defaultGeneratedNamePrefix = "tf"

var once sync.Once

type Config struct {
	Version    string
	Commit     string
	HTTPClient *http.Client

	GeneratedNamePrefix string
}

func New(c *Config) func() *schema.Provider {
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
					Type:        schema.TypeString,
					Required:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("KATAPULT_API_KEY", nil),
					Description: "API Key for Katapult Core API. Can be " +
						"specified with the `KATAPULT_API_KEY` environment " +
						"variable.",
				},
				"organization": {
					Type:     schema.TypeString,
					Required: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_ORGANIZATION", nil,
					),
					Description: "Organization sub-domain. Can be " +
						"specified with the `KATAPULT_ORGANIZATION` " +
						"environment variable.",
				},
				"data_center": {
					Type:     schema.TypeString,
					Required: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_DATA_CENTER", nil,
					),
					Description: "Data center permalink. Can be " +
						"specified with the `KATAPULT_DATA_CENTER` " +
						"environment variable.",
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
				"katapult_ip":                    resourceIP(),
				"katapult_virtual_machine":       resourceVirtualMachine(),
				"katapult_virtual_machine_group": resourceVirtualMachineGroup(),
			},
			//nolint:lll
			DataSourcesMap: map[string]*schema.Resource{
				"katapult_data_center":              dataSourceDataCenter(),
				"katapult_disk_template":            dataSourceDiskTemplate(),
				"katapult_disk_templates":           dataSourceDiskTemplates(),
				"katapult_ip":                       dataSourceIP(),
				"katapult_network_speed_profile":    dataSourceNetworkSpeedProfile(),
				"katapult_network_speed_profiles":   dataSourceNetworkSpeedProfiles(),
				"katapult_virtual_machine":          dataSourceVirtualMachine(),
				"katapult_virtual_machine_group":    dataSourceVirtualMachineGroup(),
				"katapult_virtual_machine_groups":   dataSourceVirtualMachineGroups(),
				"katapult_virtual_machine_package":  dataSourceVirtualMachinePackage(),
				"katapult_virtual_machine_packages": dataSourceVirtualMachinePackages(),
			},
		}

		p.ConfigureContextFunc = configure(c, p)

		return p
	}
}

func configure(
	conf *Config,
	p *schema.Provider,
) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(
		ctx context.Context,
		d *schema.ResourceData,
	) (interface{}, diag.Diagnostics) {
		m := &Meta{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:       "katapult",
				Level:      hclog.LevelFromString(d.Get("log_level").(string)),
				TimeFormat: "2006/01/02 15:04:05",
			}),
			confAPIKey:          d.Get("api_key").(string),
			confDataCenter:      d.Get("data_center").(string),
			confOrganization:    d.Get("organization").(string),
			GeneratedNamePrefix: conf.GeneratedNamePrefix,
		}

		if m.GeneratedNamePrefix == "" {
			m.GeneratedNamePrefix = defaultGeneratedNamePrefix
		}

		opts := []katapult.Opt{
			katapult.WithAPIKey(m.confAPIKey),
			katapult.WithUserAgent(
				p.UserAgent("terraform-provider-katapult", conf.Version),
			),
		}

		if conf.HTTPClient != nil {
			opts = append(opts, katapult.WithHTTPClient(conf.HTTPClient))
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

		rhc := newRetryableHTTPClient(conf, c.HTTPClient, m.Logger)
		c.HTTPClient = rhc.StandardClient()

		m.Client = c
		m.Core = core.New(m.Client)

		m.OrganizationRef = core.OrganizationRef{
			SubDomain: d.Get("organization").(string),
		}
		m.DataCenterRef = core.DataCenterRef{
			Permalink: d.Get("data_center").(string),
		}

		return m, nil
	}
}

func newRetryableHTTPClient(
	conf *Config,
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
