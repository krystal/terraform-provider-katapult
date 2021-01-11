// Package provider is a internal package containing the Katapult Terraform
// provider.
package provider

import (
	"context"
	"net/http"
	"net/url"

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
				"api_url": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("KATAPULT_API_URL", nil),
				},
				"api_key": {
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("KATAPULT_API_KEY", nil),
				},
				"organization_id": {
					Type:     schema.TypeString,
					Required: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_ORGANIZATION_ID", nil,
					),
				},
				"data_center_id": {
					Type:     schema.TypeString,
					Required: true,
					DefaultFunc: schema.EnvDefaultFunc(
						"KATAPULT_DATA_CENTER_ID", nil,
					),
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"katapult_ip":              resourceIP(),
				"katapult_load_balancer":   resourceLoadBalancer(),
				"katapult_virtual_machine": resourceVirtualMachine(),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"katapult_data_center":     dataSourceDataCenter(),
				"katapult_disk_template":   dataSourceDiskTemplate(),
				"katapult_disk_templates":  dataSourceDiskTemplates(),
				"katapult_ip":              dataSourceIP(),
				"katapult_load_balancer":   dataSourceLoadBalancer(),
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
			APIURL:              d.Get("api_url").(string),
			APIKey:              d.Get("api_key").(string),
			OrganizationID:      d.Get("organization_id").(string),
			DataCenterID:        d.Get("data_center_id").(string),
			GeneratedNamePrefix: conf.GeneratedNamePrefix,
		}

		if m.GeneratedNamePrefix == "" {
			m.GeneratedNamePrefix = defaultGeneratedNamePrefix
		}

		if m.APIKey == "" {
			return m, diag.Diagnostics{
				{
					Severity: diag.Error,
					Summary:  "No API KEY provided",
					Detail: "No API KEY provided. Please set \"api_key\" " +
						"provider argument, or KATAPULT_API_KEY environment " +
						"variable.",
				},
			}
		}

		c, err := katapult.NewClient(&katapult.Config{
			APIKey:    m.APIKey,
			UserAgent: p.UserAgent("terraform-provider-katapult", conf.Version),
		})
		if err != nil {
			return m, diag.FromErr(err)
		}
		m.Client = c

		if conf.Transport != nil {
			err := c.SetTransport(conf.Transport)
			if err != nil {
				return m, diag.FromErr(err)
			}
		}

		if m.APIURL != "" {
			u, err := url.Parse(m.APIURL)
			if err != nil {
				return m, diag.FromErr(err)
			}

			err = c.SetBaseURL(u)
			if err != nil {
				return m, diag.FromErr(err)
			}
		}

		return m, nil
	}
}
