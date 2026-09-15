package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

const certificatesDataSourcePageSize = 100

type (
	CertificatesDataSource struct {
		M *Meta
	}

	CertificatesDataSourceModel struct {
		Certificates types.List `tfsdk:"certificates"`
	}
)

var certificatesDataSourceItemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":             types.StringType,
		"name":           types.StringType,
		"issuer":         types.StringType,
		"state":          types.StringType,
		"expires_at":     types.StringType,
		"last_issued_at": types.StringType,
	},
}

func (r CertificatesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_certificates"
}

func (r *CertificatesDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	r.M = meta
}

func (r CertificatesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every certificate in the organization. " +
			"The list endpoint returns summary fields only; use the " +
			"`katapult_certificate` data source for full details.",
		Attributes: map[string]schema.Attribute{
			"certificates": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Certificates in the organization.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID of the certificate.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The primary hostname.",
						},
						"issuer": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The certificate issuer: " +
								"`lets_encrypt`, `custom`, or `self_signed`.",
						},
						"state": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The certificate state: " +
								"`pending`, `issuing`, `issued`, or " +
								"`issue_failed`.",
						},
						"expires_at": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "When the certificate " +
								"expires, as an RFC 3339 UTC timestamp. Null " +
								"until issued.",
						},
						"last_issued_at": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "When the certificate was " +
								"last issued, as an RFC 3339 UTC timestamp. " +
								"Null until issued.",
						},
					},
				},
			},
		},
	}
}

func (r CertificatesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data CertificatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	certs := []core.GetOrganizationCertificates200ResponseCertificates{}
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		res, err := r.M.Core.GetOrganizationCertificatesWithResponse(ctx,
			&core.GetOrganizationCertificatesParams{
				OrganizationSubDomain: &r.M.confOrganization,
				Page:                  ptr(page),
				PerPage:               ptr(certificatesDataSourcePageSize),
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}

			resp.Diagnostics.AddError("Certificates Error", err.Error())
			return
		}

		if pages, err := res.JSON200.Pagination.TotalPages.Get(); err == nil {
			totalPages = pages
		}

		certs = append(certs, res.JSON200.Certificates...)
	}

	items := make([]attr.Value, len(certs))
	for i, cert := range certs {
		issuer := types.StringNull()
		if cert.Issuer != nil {
			issuer = types.StringValue(string(*cert.Issuer))
		}
		state := types.StringNull()
		if cert.State != nil {
			state = types.StringValue(string(*cert.State))
		}

		items[i] = types.ObjectValueMust(
			certificatesDataSourceItemType.AttrTypes,
			map[string]attr.Value{
				"id":             types.StringPointerValue(cert.Id),
				"name":           types.StringPointerValue(cert.Name),
				"issuer":         issuer,
				"state":          state,
				"expires_at":     nullableUnixSecondsValue(cert.ExpiresAt),
				"last_issued_at": nullableUnixSecondsValue(cert.LastIssuedAt),
			},
		)
	}

	data.Certificates = types.ListValueMust(certificatesDataSourceItemType, items)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
