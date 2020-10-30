// Package provider is a internal package containing the Katapult Terraform
// provider.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult"
)

type Config struct {
	Version   string
	Commit    string
	Date      string
	Transport http.RoundTripper
}

type Meta struct {
	Client *katapult.Client
	Ctx    context.Context

	APIURL         string
	APIKey         string
	OrganizationID string
	DataCenterID   string
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
				"katapult_load_balancer": resourceLoadBalancer(),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"katapult_data_center":   dataSourceDataCenter(),
				"katapult_load_balancer": dataSourceLoadBalancer(),
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
			Ctx:            ctx,
			APIURL:         d.Get("api_url").(string),
			APIKey:         d.Get("api_key").(string),
			OrganizationID: d.Get("organization_id").(string),
			DataCenterID:   d.Get("data_center_id").(string),
		}

		var diags diag.Diagnostics

		if m.APIKey == "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "No API KEY provided",
				Detail: "No API KEY provided. Please set \"api_key\" " +
					"provider argument, or KATAPULT_API_KEY environment " +
					"variable.",
			})

			return m, diags
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
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "Failed to parse API URL",
					Detail: fmt.Sprintf(
						"Failed to parse API URL: %s",
						m.APIURL,
					),
				})

				return nil, diags
			}

			err = c.SetBaseURL(u)
			if err != nil {
				return nil, diag.FromErr(err)
			}
		}

		return m, nil
	}
}
