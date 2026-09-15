package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	CertificateDataSource struct {
		M *Meta
	}

	CertificateDataSourceModel struct {
		CertificateModel

		AuthorizationMethod types.String `tfsdk:"authorization_method"`
	}
)

func (r CertificateDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *CertificateDataSource) Configure(
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

func (r CertificateDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	computed := func(sensitive bool, description string) schema.Attribute {
		return schema.StringAttribute{
			Computed:            true,
			Sensitive:           sensitive,
			MarkdownDescription: description,
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches details of a certificate by ID, " +
			"regardless of its issuer.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the certificate.",
			},
			"name": computed(false, "The primary hostname of the certificate."),
			"additional_names": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Additional hostnames in the certificate.",
			},
			"authorization_method": computed(false,
				"The Let's Encrypt authorization method, `dns` or `http`. "+
					"Null for other issuers.",
			),
			"issuer": computed(false,
				"The certificate issuer: `lets_encrypt`, `custom`, or "+
					"`self_signed`.",
			),
			"state": computed(false,
				"The certificate state: `pending`, `issuing`, `issued`, or "+
					"`issue_failed`.",
			),
			"issue_error": computed(false,
				"The error from the last failed issuance attempt. Null unless "+
					"the certificate is in the `issue_failed` state.",
			),
			"certificate": computed(false,
				"The PEM encoded certificate. Null until issued.",
			),
			"chain": computed(false,
				"The PEM encoded certificate chain. Null when there is no chain.",
			),
			"private_key": computed(true,
				"The PEM encoded private key. Null until issued, or when the "+
					"API token cannot view private certificate material.",
			),
			"certificate_api_url": computed(true,
				"URL of the certificate API endpoint that serves this "+
					"certificate's material. It embeds a bearer token, so treat "+
					"it as a secret. Null when the API token cannot view private "+
					"certificate material.",
			),
			"created_at": computed(false,
				"When the certificate was created, as an RFC 3339 UTC timestamp.",
			),
			"expires_at": computed(false,
				"When the certificate expires, as an RFC 3339 UTC timestamp. "+
					"Null until issued.",
			),
			"last_issued_at": computed(false,
				"When the certificate was last issued, as an RFC 3339 UTC "+
					"timestamp. Null until issued.",
			),
		},
	}
}

func (r CertificateDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data CertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cert, err := getCertificate(ctx, r.M, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Certificate Error", err.Error())
		return
	}

	data.FromAPI(cert)
	data.AuthorizationMethod = nullableStringValue(cert.AuthorizationMethod)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
