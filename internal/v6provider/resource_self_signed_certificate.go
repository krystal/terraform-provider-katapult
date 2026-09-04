package v6provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/krystal/go-katapult/next/core"
)

const (
	selfSignedCertificateCreateTimeout = 5 * time.Minute
	certificateDeleteTimeout           = 2 * time.Minute
)

type (
	SelfSignedCertificateResource struct {
		M *Meta
	}

	SelfSignedCertificateResourceModel struct {
		CertificateModel

		Timeouts timeouts.Value `tfsdk:"timeouts"`
	}
)

var (
	_ resource.ResourceWithConfigure   = (*SelfSignedCertificateResource)(nil)
	_ resource.ResourceWithImportState = (*SelfSignedCertificateResource)(nil)
)

func (r *SelfSignedCertificateResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_self_signed_certificate"
}

func (r *SelfSignedCertificateResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
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

func (r SelfSignedCertificateResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	attributes := certificateComputedAttributes()

	attributes["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The ID of the certificate.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	attributes["name"] = schema.StringAttribute{
		Required: true,
		MarkdownDescription: "The primary hostname of the certificate. " +
			"Hostnames must be lowercase with a dotted suffix, for example " +
			"`app.internal.example.com`. Changing this replaces the " +
			"certificate.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["additional_names"] = certificateAdditionalNamesAttribute()
	attributes["timeouts"] = timeouts.Attributes(ctx, timeouts.Opts{
		Create: true,
		Delete: true,
	})

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a self-signed certificate in Katapult. " +
			"Katapult issues an RSA 4096 certificate valid for one year and " +
			"re-issues it automatically a month before expiry. Create waits " +
			"for the initial issuance to complete.\n\n" +
			"Every configurable argument replaces the certificate when " +
			"changed. Deleting a certificate fails while a load balancer " +
			"rule references it, so set `lifecycle { create_before_destroy " +
			"= true }` when rotating certificates that are attached to rules.",
		Attributes: attributes,
	}
}

func (r *SelfSignedCertificateResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan SelfSignedCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(
		ctx, selfSignedCertificateCreateTimeout,
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.CertificateArguments{
		Issuer: core.SelfSigned,
		Name:   plan.Name.ValueStringPointer(),
	}
	names, diags := certificateAdditionalNamesArgument(ctx, plan.AdditionalNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	args.AdditionalNames = names

	cert, task, err := createCertificate(ctx, r.M, args)
	if err != nil {
		resp.Diagnostics.AddError("Certificate Create Error", err.Error())
		return
	}

	// Persist the ID before waiting so a failed issuance taints the resource
	// instead of orphaning the certificate.
	plan.FromAPI(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	issued, err := waitForCertificateIssuance(
		ctx, r.M, plan.ID.ValueString(), task, createTimeout,
	)
	if issued != nil {
		plan.FromAPI(issued)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Certificate Issuance Error", err.Error())
	}
}

func (r *SelfSignedCertificateResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state SelfSignedCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cert, err := getCertificate(ctx, r.M, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			r.M.Logger.Info(
				"Certificate not found, removing from state",
				"id", state.ID.ValueString(),
			)
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Certificate Read Error", err.Error())
		return
	}

	state.FromAPI(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only persists timeouts changes. Every other argument requires
// replacement because the API has no certificate update endpoint.
func (r *SelfSignedCertificateResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan SelfSignedCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *SelfSignedCertificateResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state SelfSignedCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(
		ctx, certificateDeleteTimeout,
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if err := deleteCertificate(ctx, r.M, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Certificate Delete Error", err.Error())
	}
}

func (r *SelfSignedCertificateResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	cert, err := getCertificate(ctx, r.M, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	if err := checkCertificateIssuer(cert, core.SelfSigned); err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
