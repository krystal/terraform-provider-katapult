package v6provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

const (
	letsEncryptCertificateCreateTimeout = 10 * time.Minute

	certificateAuthorizationMethodDNS  = "dns"
	certificateAuthorizationMethodHTTP = "http"
)

type (
	LetsEncryptCertificateResource struct {
		M *Meta
	}

	LetsEncryptCertificateResourceModel struct {
		CertificateModel

		AuthorizationMethod types.String   `tfsdk:"authorization_method"`
		WaitForIssuance     types.Bool     `tfsdk:"wait_for_issuance"`
		Timeouts            timeouts.Value `tfsdk:"timeouts"`
	}
)

var (
	_ resource.ResourceWithConfigure      = (*LetsEncryptCertificateResource)(nil)
	_ resource.ResourceWithImportState    = (*LetsEncryptCertificateResource)(nil)
	_ resource.ResourceWithValidateConfig = (*LetsEncryptCertificateResource)(nil)
)

func (r *LetsEncryptCertificateResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_lets_encrypt_certificate"
}

func (r *LetsEncryptCertificateResource) Configure(
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

//nolint:lll // Prerequisite documentation reads better as full paragraphs.
func (r LetsEncryptCertificateResource) Schema(
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
			"`www.example.com`. Wildcards are written as `*.example.com` and " +
			"require `dns` authorization. Changing this replaces the " +
			"certificate.",
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["additional_names"] = certificateAdditionalNamesAttribute()
	attributes["authorization_method"] = schema.StringAttribute{
		Required: true,
		MarkdownDescription: "How Let's Encrypt validates control of the " +
			"names: `dns` or `http`. Changing this replaces the certificate.",
		Validators: []validator.String{
			stringvalidator.OneOf(
				certificateAuthorizationMethodDNS,
				certificateAuthorizationMethodHTTP,
			),
		},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["wait_for_issuance"] = schema.BoolAttribute{
		Optional: true,
		Computed: true,
		Default:  booldefault.StaticBool(true),
		MarkdownDescription: "Wait for the certificate to be issued during " +
			"create. Defaults to `true`. Set to `false` when DNS is cut over " +
			"to the load balancer outside Terraform; create then returns " +
			"once the certificate exists in the `pending` state and " +
			"Katapult keeps retrying issuance on its own schedule.",
	}
	attributes["timeouts"] = timeouts.Attributes(ctx, timeouts.Opts{
		Create: true,
		Delete: true,
	})

	resp.Schema = schema.Schema{
		MarkdownDescription: strings.TrimSpace(`
Manages a Let's Encrypt certificate issued through Katapult. Katapult issues the certificate as a background task and renews it automatically.

Issuance depends on prerequisites this resource cannot create.

With ` + "`authorization_method = \"dns\"`" + `, every name, or a parent domain of it, must be in a verified Katapult DNS zone. Katapult creates the ` + "`_acme-challenge`" + ` records in its own DNS service and removes them after validation. Domains hosted outside Katapult DNS cannot use ` + "`dns`" + ` authorization. Wildcard names require ` + "`dns`" + `.

With ` + "`authorization_method = \"http\"`" + `, every name must resolve to a Katapult load balancer in the same organization that listens on port 80, either through an HTTP ` + "`katapult_load_balancer_rule`" + ` with ` + "`listen_port = 80`" + ` or through a load balancer with ` + "`https_redirect`" + ` enabled. Katapult answers the ` + "`/.well-known/acme-challenge/*`" + ` requests itself, so the certificate does not need to be attached to a rule before it is issued. The port-80 listener can be managed in the same configuration; declare ` + "`depends_on`" + ` for it on the certificate so one apply creates the listener, issues the certificate, and attaches it to an HTTPS rule.

Create waits for issuance by default. Set ` + "`wait_for_issuance = false`" + ` only when DNS is pointed at the load balancer outside Terraform; the certificate is created in the ` + "`pending`" + ` state and Katapult retries issuance on its own schedule.

Every configurable argument other than ` + "`wait_for_issuance`" + ` and ` + "`timeouts`" + ` replaces the certificate when changed. Deleting a certificate fails while a load balancer rule references it, so set ` + "`lifecycle { create_before_destroy = true }`" + ` when rotating certificates that are attached to rules.
`),
		Attributes: attributes,
	}
}

// ValidateConfig mirrors the API rule that wildcard names cannot be validated
// with HTTP authorization, so the failure surfaces at plan time.
func (r LetsEncryptCertificateResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var config LetsEncryptCertificateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.AuthorizationMethod.IsNull() ||
		config.AuthorizationMethod.IsUnknown() ||
		config.AuthorizationMethod.ValueString() !=
			certificateAuthorizationMethodHTTP {
		return
	}

	if isWildcardHostname(config.Name) {
		resp.Diagnostics.AddAttributeError(
			path.Root("name"),
			"Wildcard Requires DNS Authorization",
			"Let's Encrypt cannot validate wildcard names with http "+
				"authorization. Use authorization_method = \"dns\" for "+
				config.Name.ValueString()+".",
		)
	}

	if config.AdditionalNames.IsNull() || config.AdditionalNames.IsUnknown() {
		return
	}
	for _, element := range config.AdditionalNames.Elements() {
		name, ok := element.(types.String)
		if !ok || !isWildcardHostname(name) {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			path.Root("additional_names"),
			"Wildcard Requires DNS Authorization",
			"Let's Encrypt cannot validate wildcard names with http "+
				"authorization. Use authorization_method = \"dns\" for "+
				name.ValueString()+".",
		)
	}
}

func isWildcardHostname(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() &&
		strings.HasPrefix(value.ValueString(), "*.")
}

func (r *LetsEncryptCertificateResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan LetsEncryptCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(
		ctx, letsEncryptCertificateCreateTimeout,
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	names, diags := certificateAdditionalNamesArgument(ctx, plan.AdditionalNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.CertificateArguments{
		Issuer:              core.LetsEncrypt,
		Name:                plan.Name.ValueStringPointer(),
		AuthorizationMethod: plan.AuthorizationMethod.ValueStringPointer(),
		AdditionalNames:     names,
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

	if !plan.WaitForIssuance.ValueBool() {
		return
	}

	issued, err := waitForCertificateIssuance(
		ctx, r.M, plan.ID.ValueString(), task, createTimeout,
	)
	if issued != nil {
		plan.applyAPI(issued)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Certificate Issuance Error", err.Error())
	}
}

// applyAPI maps an API certificate into the model. authorization_method is
// configured, so the API value is only taken when the API reports one, and an
// imported wait_for_issuance materializes its default.
func (m *LetsEncryptCertificateResourceModel) applyAPI(cert *core.Certificate) {
	m.FromAPI(cert)

	if method := nullableStringValue(cert.AuthorizationMethod); !method.IsNull() {
		m.AuthorizationMethod = method
	}
	if m.WaitForIssuance.IsNull() || m.WaitForIssuance.IsUnknown() {
		m.WaitForIssuance = types.BoolValue(true)
	}
}

func (r *LetsEncryptCertificateResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state LetsEncryptCertificateResourceModel
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

// Update only persists wait_for_issuance and timeouts changes. Every other
// argument requires replacement because the API has no certificate update
// endpoint.
func (r *LetsEncryptCertificateResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan LetsEncryptCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *LetsEncryptCertificateResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state LetsEncryptCertificateResourceModel
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

func (r *LetsEncryptCertificateResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	cert, err := getCertificate(ctx, r.M, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	if err := checkCertificateIssuer(cert, core.LetsEncrypt); err != nil {
		resp.Diagnostics.AddError("Certificate Import Error", err.Error())
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
