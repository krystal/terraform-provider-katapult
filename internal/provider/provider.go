// Package provider is a internal package containing the Katapult Terraform
// provider.
package provider

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult/pkg/katapult"
)

const defaultGeneratedNamePrefix = "tf"

type Config struct {
	Version    string
	Commit     string
	HTTPClient *http.Client

	GeneratedNamePrefix string
}

func New(c *Config) func() *schema.Provider {
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
					Description: "Organization sub-domain or ID. Can be " +
						"specified with the `KATAPULT_ORGANIZATION` " +
						"environment variable.",
				},
				"data_center": {
					Type:     schema.TypeString,
					Required: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_DATA_CENTER", nil,
					),
					Description: "Data center permalink or ID. Can be " +
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
						"(default: `info`)",
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"katapult_ip":              resourceIP(),
				"katapult_virtual_machine": resourceVirtualMachine(),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"katapult_data_center":     dataSourceDataCenter(),
				"katapult_disk_template":   dataSourceDiskTemplate(),
				"katapult_disk_templates":  dataSourceDiskTemplates(),
				"katapult_ip":              dataSourceIP(),
				"katapult_virtual_machine": dataSourceVirtualMachine(),
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
			Ctx: ctx,
			Logger: hclog.New(&hclog.LoggerOptions{
				Level: hclog.LevelFromString(d.Get("log_level").(string)),
			}),

			confAPIKey:          d.Get("api_key").(string),
			confDataCenter:      d.Get("data_center").(string),
			confOrganization:    d.Get("organization").(string),
			GeneratedNamePrefix: conf.GeneratedNamePrefix,
		}

		if m.GeneratedNamePrefix == "" {
			m.GeneratedNamePrefix = defaultGeneratedNamePrefix
		}

		c, err := katapult.NewClient(&katapult.Config{
			APIKey:    m.confAPIKey,
			UserAgent: p.UserAgent("terraform-provider-katapult", conf.Version),
		})
		if err != nil {
			return m, diag.FromErr(err)
		}

		rhc := newRetryableHTTPClient(conf, m.Logger)
		err = c.SetHTTPClient(rhc.StandardClient())
		if err != nil {
			return m, diag.FromErr(err)
		}

		// Debug override of API URL for internal testing purposes.
		if apiURL := os.Getenv("KATAPULT_TF_DEBUG_API_URL"); apiURL != "" {
			u, err := url.Parse(apiURL)
			if err != nil {
				return m, diag.FromErr(err)
			}

			err = c.SetBaseURL(u)
			if err != nil {
				return m, diag.FromErr(err)
			}
		}

		m.Client = c
		m.organizationRef, _ = katapult.NewOrganizationLookup(
			d.Get("organization").(string),
		)
		m.dataCenterRef, _ = katapult.NewDataCenterLookup(
			d.Get("data_center").(string),
		)

		return m, nil
	}
}

func newRetryableHTTPClient(
	conf *Config,
	logger hclog.Logger,
) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.Logger = logger
	client.RetryWaitMin = 1 * time.Second
	client.RetryWaitMax = 2 * time.Minute
	client.RetryMax = 4
	client.CheckRetry = requestRetryPolicy

	if conf.HTTPClient != nil {
		client.HTTPClient = conf.HTTPClient
	}

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
