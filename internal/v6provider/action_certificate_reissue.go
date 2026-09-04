package v6provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

const certificateReissueDefaultTimeout = 10 * time.Minute

type (
	CertificateReissueAction struct {
		M *Meta
	}

	CertificateReissueActionModel struct {
		CertificateID types.String `tfsdk:"certificate_id"`
		Timeout       types.String `tfsdk:"timeout"`
	}
)

var (
	_ action.ActionWithConfigure      = (*CertificateReissueAction)(nil)
	_ action.ActionWithValidateConfig = (*CertificateReissueAction)(nil)
)

func (a *CertificateReissueAction) Metadata(
	_ context.Context,
	req action.MetadataRequest,
	resp *action.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_certificate_reissue"
}

func (a *CertificateReissueAction) Configure(
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

func (a *CertificateReissueAction) Schema(
	_ context.Context,
	_ action.SchemaRequest,
	resp *action.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Requests a new issuance of a Let's Encrypt or " +
			"self-signed certificate and waits for the issuance task to " +
			"complete. Custom certificates cannot be re-issued.\n\n" +
			"The owning certificate resource's `certificate`, `expires_at`, " +
			"`last_issued_at`, and `certificate_api_url` attributes refresh " +
			"on the next plan, not within the apply that ran the action.",
		Attributes: map[string]schema.Attribute{
			"certificate_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the certificate to re-issue.",
			},
			"timeout": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "How long to wait for the issuance task, " +
					"as a Go duration such as `10m`. Defaults to `10m`.",
			},
		},
	}
}

func (a *CertificateReissueAction) ValidateConfig(
	ctx context.Context,
	req action.ValidateConfigRequest,
	resp *action.ValidateConfigResponse,
) {
	var config CertificateReissueActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := parseCertificateReissueTimeout(config.Timeout); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("timeout"), "Invalid Timeout", err.Error(),
		)
	}
}

func parseCertificateReissueTimeout(value types.String) (time.Duration, error) {
	if value.IsNull() || value.IsUnknown() {
		return certificateReissueDefaultTimeout, nil
	}

	timeout, err := time.ParseDuration(value.ValueString())
	if err != nil {
		return 0, fmt.Errorf(
			"timeout must be a Go duration such as \"10m\": %w", err,
		)
	}
	if timeout <= 0 {
		return 0, errors.New("timeout must be greater than zero")
	}

	return timeout, nil
}

func (a *CertificateReissueAction) Invoke(
	ctx context.Context,
	req action.InvokeRequest,
	resp *action.InvokeResponse,
) {
	var config CertificateReissueActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, err := parseCertificateReissueTimeout(config.Timeout)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("timeout"), "Invalid Timeout", err.Error(),
		)
		return
	}

	id := config.CertificateID.ValueString()
	res, err := a.M.Core.PostCertificateIssueWithResponse(ctx,
		core.PostCertificateIssueJSONRequestBody{
			Certificate: core.CertificateLookup{Id: &id},
		})
	if err != nil {
		if res != nil && (res.StatusCode() == http.StatusUnprocessableEntity ||
			res.JSON422 != nil) {
			resp.Diagnostics.AddError(
				"Certificate Reissue Not Supported",
				fmt.Sprintf(
					"Certificate %s cannot be re-issued. Custom certificates "+
						"are uploaded as-is; replace the "+
						"katapult_custom_certificate resource with new "+
						"material instead (%s).",
					id, genericAPIError(err, res.Body),
				),
			)
			return
		}
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		resp.Diagnostics.AddError("Certificate Reissue Error", err.Error())
		return
	}
	if res.JSON200 == nil || res.JSON200.Task.Id == nil {
		resp.Diagnostics.AddError(
			"Certificate Reissue Error",
			"unexpected empty response requesting certificate issuance",
		)
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf(
				"Waiting for certificate %s issuance task %s",
				id, *res.JSON200.Task.Id,
			),
		})
	}

	if err := waitForTaskCompletion(
		ctx, a.M, timeout, *res.JSON200.Task.Id,
	); err != nil {
		resp.Diagnostics.AddError(
			"Certificate Reissue Error",
			fmt.Sprintf("Certificate %s issuance did not complete: %s",
				id, err.Error()),
		)
	}
}
