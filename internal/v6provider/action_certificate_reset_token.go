package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	CertificateResetTokenAction struct {
		M *Meta
	}

	CertificateResetTokenActionModel struct {
		CertificateID types.String `tfsdk:"certificate_id"`
	}
)

var _ action.ActionWithConfigure = (*CertificateResetTokenAction)(nil)

func (a *CertificateResetTokenAction) Metadata(
	_ context.Context,
	req action.MetadataRequest,
	resp *action.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_certificate_reset_token"
}

func (a *CertificateResetTokenAction) Configure(
	_ context.Context,
	req action.ConfigureRequest,
	resp *action.ConfigureResponse,
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

	a.M = meta
}

func (a *CertificateResetTokenAction) Schema(
	_ context.Context,
	_ action.SchemaRequest,
	resp *action.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Rotates the bearer token embedded in a " +
			"certificate's `certificate_api_url`, invalidating the previous " +
			"URL.\n\n" +
			"The owning certificate resource's `certificate_api_url` " +
			"attribute refreshes on the next plan, not within the apply that " +
			"ran the action.",
		Attributes: map[string]schema.Attribute{
			"certificate_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The ID of the certificate whose API " +
					"token to reset.",
			},
		},
	}
}

func (a *CertificateResetTokenAction) Invoke(
	ctx context.Context,
	req action.InvokeRequest,
	resp *action.InvokeResponse,
) {
	var config CertificateResetTokenActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.CertificateID.ValueString()
	res, err := a.M.Core.PostCertificateResetTokenWithResponse(ctx,
		core.PostCertificateResetTokenJSONRequestBody{
			Certificate: core.CertificateLookup{Id: &id},
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		resp.Diagnostics.AddError("Certificate Reset Token Error", err.Error())
	}
}
