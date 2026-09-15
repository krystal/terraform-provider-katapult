package v6provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

// customCertificateCreateTimeout bounds the wait when the API unexpectedly
// returns an issuance task for a custom certificate, which is normally issued
// immediately with a null task.
const customCertificateCreateTimeout = 5 * time.Minute

type (
	CustomCertificateResource struct {
		M *Meta
	}

	CustomCertificateResourceModel struct {
		CertificateModel

		Timeouts timeouts.Value `tfsdk:"timeouts"`
	}
)

var (
	_ resource.ResourceWithConfigure   = (*CustomCertificateResource)(nil)
	_ resource.ResourceWithImportState = (*CustomCertificateResource)(nil)
)

func (r *CustomCertificateResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_custom_certificate"
}

func (r *CustomCertificateResource) Configure(
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

func (r CustomCertificateResource) Schema(
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
		Computed: true,
		MarkdownDescription: "The primary hostname, derived by Katapult " +
			"from the certificate's common name.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
	attributes["additional_names"] = schema.SetAttribute{
		Computed:    true,
		ElementType: types.StringType,
		MarkdownDescription: "Additional hostnames, derived by Katapult " +
			"from the certificate's subject alternative names.",
		PlanModifiers: []planmodifier.Set{
			setplanmodifier.UseStateForUnknown(),
		},
	}
	attributes["certificate"] = schema.StringAttribute{
		Required: true,
		MarkdownDescription: "The PEM encoded certificate. Expired " +
			"certificates are rejected. Changing this replaces the " +
			"certificate.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["private_key"] = schema.StringAttribute{
		Required:  true,
		Sensitive: true,
		MarkdownDescription: "The PEM encoded RSA private key. Changing " +
			"this replaces the certificate.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["chain"] = schema.StringAttribute{
		Optional: true,
		MarkdownDescription: "The PEM encoded certificate chain. Changing " +
			"this replaces the certificate.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["timeouts"] = timeouts.Attributes(ctx, timeouts.Opts{
		Delete: true,
	})

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom certificate uploaded to " +
			"Katapult. Katapult derives `name` and `additional_names` from " +
			"the certificate and issues it immediately.\n\n" +
			"The configured `certificate`, `private_key`, and `chain` " +
			"values are kept in state as written and are not refreshed " +
			"from the API. Import reads them from the API once. After " +
			"import, the configured `certificate`, `private_key`, and " +
			"`chain` must match the imported values or the first plan " +
			"replaces the certificate; when the API token cannot view " +
			"private certificate material the imported `private_key` is " +
			"null and the first plan replaces it.\n\n" +
			"Every configurable argument other than `timeouts` replaces " +
			"the certificate when changed. Deleting a certificate fails " +
			"while a load balancer rule references it, so set `lifecycle { " +
			"create_before_destroy = true }` when rotating certificates " +
			"that are attached to rules.",
		Attributes: attributes,
	}
}

// applyAPI maps an API certificate into the model while keeping the PEM
// inputs exactly as stored, including a null chain, so server-side
// normalization and redacted replay responses never produce a diff. Only
// ImportState fills the PEM inputs from the API.
func (m *CustomCertificateResourceModel) applyAPI(cert *core.Certificate) {
	certificate, privateKey, chain := m.Certificate, m.PrivateKey, m.Chain

	m.FromAPI(cert)

	m.Certificate, m.PrivateKey, m.Chain = certificate, privateKey, chain
}

// customCertificateNullTimeouts returns a null timeouts value with the
// attribute types of the resource schema, for state written without a
// configuration such as import.
func customCertificateNullTimeouts(ctx context.Context) timeouts.Value {
	attrType := timeouts.Attributes(ctx, timeouts.Opts{Delete: true}).GetType()
	objectType, ok := attrType.(attr.TypeWithAttributeTypes)
	if !ok {
		return timeouts.Value{Object: types.ObjectNull(nil)}
	}

	return timeouts.Value{
		Object: types.ObjectNull(objectType.AttributeTypes()),
	}
}

func (r *CustomCertificateResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan CustomCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.CertificateArguments{
		Issuer:      core.Custom,
		Certificate: plan.Certificate.ValueStringPointer(),
		PrivateKey:  plan.PrivateKey.ValueStringPointer(),
	}
	if !plan.Chain.IsNull() && !plan.Chain.IsUnknown() {
		args.Chain = plan.Chain.ValueStringPointer()
	}

	cert, task, err := createCertificate(ctx, r.M, args)
	if err != nil {
		resp.Diagnostics.AddError("Certificate Create Error", err.Error())
		return
	}

	// Persist the ID before waiting so a failed issuance taints the resource
	// instead of orphaning the certificate.
	plan.applyAPI(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	issued, err := waitForCertificateIssuance(
		ctx, r.M, plan.ID.ValueString(), task, customCertificateCreateTimeout,
	)
	if issued != nil {
		plan.applyAPI(issued)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Certificate Issuance Error", err.Error())
	}
}

func (r *CustomCertificateResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state CustomCertificateResourceModel
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

	state.applyAPI(cert)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only persists timeouts changes. Every other argument requires
// replacement because the API has no certificate update endpoint.
func (r *CustomCertificateResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan CustomCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *CustomCertificateResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state CustomCertificateResourceModel
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

func (r *CustomCertificateResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	cert, err := getCertificate(ctx, r.M, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	if err := checkCertificateIssuer(cert, core.Custom); err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	// Import is the one time the PEM inputs are read from the API. Later
	// reads keep whatever state holds, so the imported values must match the
	// configuration or the first plan replaces the certificate.
	model := CustomCertificateResourceModel{
		Timeouts: customCertificateNullTimeouts(ctx),
	}
	model.FromAPI(cert)

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
