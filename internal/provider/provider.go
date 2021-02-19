// Package provider is a internal package containing the Katapult Terraform
// provider.
package provider

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

const defaultGeneratedNamePrefix = "tf"

type Config struct {
	Version             string
	Commit              string
	Date                string
	Transport           http.RoundTripper
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
			Ctx:                 ctx,
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

		if conf.Transport != nil {
			err := c.SetTransport(conf.Transport)
			if err != nil {
				return m, diag.FromErr(err)
			}
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
