package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/oapi-codegen/nullable"
)

// CertificateModel holds the attributes shared by every certificate resource
// and the singular certificate data source. Resource models embed it by value
// alongside their own arguments.
type CertificateModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	AdditionalNames   types.Set    `tfsdk:"additional_names"`
	Issuer            types.String `tfsdk:"issuer"`
	State             types.String `tfsdk:"state"`
	IssueError        types.String `tfsdk:"issue_error"`
	Certificate       types.String `tfsdk:"certificate"`
	Chain             types.String `tfsdk:"chain"`
	PrivateKey        types.String `tfsdk:"private_key"`
	CertificateAPIURL types.String `tfsdk:"certificate_api_url"`
	CreatedAt         types.String `tfsdk:"created_at"`
	ExpiresAt         types.String `tfsdk:"expires_at"`
	LastIssuedAt      types.String `tfsdk:"last_issued_at"`
}

// certificateResourceTypeNames maps API issuers to the Terraform resource type
// that manages certificates with that issuer.
var certificateResourceTypeNames = map[core.IssuerEnum]string{
	core.LetsEncrypt: providerTypeName + "_lets_encrypt_certificate",
	core.SelfSigned:  providerTypeName + "_self_signed_certificate",
	core.Custom:      providerTypeName + "_custom_certificate",
}

func ConvertCoreCertsToTFValues(
	certs []core.GetLoadBalancersRulesLoadBalancerRulePartCertificates,
) []attr.Value {
	values := make([]attr.Value, len(certs))
	for i, cert := range certs {
		values[i] = types.StringPointerValue(cert.Id)
	}
	return values
}

// FromAPI copies every shared attribute from the API certificate. Null API
// values stay null in state.
func (m *CertificateModel) FromAPI(cert *core.Certificate) {
	m.ID = types.StringPointerValue(cert.Id)
	m.Name = types.StringPointerValue(cert.Name)
	m.AdditionalNames = stringSliceToSet(cert.AdditionalNames)
	m.Issuer = types.StringNull()
	if cert.Issuer != nil {
		m.Issuer = types.StringValue(string(*cert.Issuer))
	}
	m.State = types.StringNull()
	if cert.State != nil {
		m.State = types.StringValue(string(*cert.State))
	}
	m.IssueError = nullableStringValue(cert.IssueError)
	m.Certificate = nullableStringValue(cert.Certificate)
	m.Chain = nullableStringValue(cert.Chain)
	m.PrivateKey = nullableStringValue(cert.PrivateKey)
	m.CertificateAPIURL = nullableStringValue(cert.CertificateApiUrl)
	m.CreatedAt = types.StringNull()
	if cert.CreatedAt != nil {
		m.CreatedAt = types.StringValue(unixSecondsToRFC3339(*cert.CreatedAt))
	}
	m.ExpiresAt = nullableUnixSecondsValue(cert.ExpiresAt)
	m.LastIssuedAt = nullableUnixSecondsValue(cert.LastIssuedAt)
}

// certificateComputedAttributes returns the shared computed outputs of every
// certificate resource. Resources that accept PEM input replace the
// certificate, chain, and private_key entries with configured attributes.
func certificateComputedAttributes() map[string]schema.Attribute {
	computedString := func(sensitive bool, description string) schema.Attribute {
		return schema.StringAttribute{
			Computed:            true,
			Sensitive:           sensitive,
			MarkdownDescription: description,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}

	return map[string]schema.Attribute{
		"issuer": computedString(false,
			"The certificate issuer: `lets_encrypt`, `custom`, or "+
				"`self_signed`.",
		),
		"state": computedString(false,
			"The certificate state: `pending`, `issuing`, `issued`, or "+
				"`issue_failed`.",
		),
		"issue_error": computedString(false,
			"The error from the last failed issuance attempt. Null unless "+
				"the certificate is in the `issue_failed` state.",
		),
		"certificate": computedString(false,
			"The PEM encoded certificate. Null until issued.",
		),
		"chain": computedString(false,
			"The PEM encoded certificate chain. Null when there is no chain.",
		),
		"private_key": computedString(true,
			"The PEM encoded private key. Null until issued, or when the "+
				"API token cannot view private certificate material.",
		),
		"certificate_api_url": computedString(true,
			"URL of the certificate API endpoint that serves this "+
				"certificate's material. It embeds a bearer token, so treat "+
				"it as a secret. Null when the API token cannot view private "+
				"certificate material.",
		),
		"created_at": computedString(false,
			"When the certificate was created, as an RFC 3339 UTC timestamp.",
		),
		"expires_at": computedString(false,
			"When the certificate expires, as an RFC 3339 UTC timestamp. "+
				"Null until issued.",
		),
		"last_issued_at": computedString(false,
			"When the certificate was last issued, as an RFC 3339 UTC "+
				"timestamp. Null until issued.",
		),
	}
}

// certificateAdditionalNamesAttribute returns the configurable
// additional_names attribute shared by the resources that issue certificates
// for configured hostnames. An omitted value plans as an empty set, so
// removing previously configured names is a change that replaces the
// certificate.
func certificateAdditionalNamesAttribute() schema.SetAttribute {
	return schema.SetAttribute{
		Optional:    true,
		Computed:    true,
		ElementType: types.StringType,
		Default: setdefault.StaticValue(
			types.SetValueMust(types.StringType, []attr.Value{}),
		),
		MarkdownDescription: "Additional hostnames to include in the " +
			"certificate. Defaults to an empty set. Adding, changing, or " +
			"removing names replaces the certificate.",
		Validators: []validator.Set{
			setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
		},
		PlanModifiers: []planmodifier.Set{
			setplanmodifier.RequiresReplace(),
		},
	}
}

// certificateAdditionalNamesArgument converts a planned additional_names set
// into the create request argument, omitting it when null, unknown, or empty.
func certificateAdditionalNamesArgument(
	ctx context.Context,
	set types.Set,
) (*[]string, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() || len(set.Elements()) == 0 {
		return nil, nil
	}

	names, diags := stringSetValueStrings(ctx, "additional_names", set)
	if diags.HasError() {
		return nil, diags
	}

	return &names, diags
}

func nullableStringValue(value nullable.Nullable[string]) types.String {
	if !value.IsSpecified() || value.IsNull() {
		return types.StringNull()
	}

	return types.StringValue(value.MustGet())
}

func nullableUnixSecondsValue(value nullable.Nullable[int]) types.String {
	if !value.IsSpecified() || value.IsNull() {
		return types.StringNull()
	}

	return types.StringValue(unixSecondsToRFC3339(value.MustGet()))
}

// unixSecondsToRFC3339 formats a unix timestamp as RFC 3339 in UTC.
func unixSecondsToRFC3339(seconds int) string {
	return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
}

func stringSliceToSet(values *[]string) types.Set {
	if values == nil {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}

	elements := make([]attr.Value, 0, len(*values))
	for _, value := range *values {
		elements = append(elements, types.StringValue(value))
	}

	return types.SetValueMust(types.StringType, elements)
}

// getCertificate fetches a certificate by ID. The generated client declares
// the GET response "certificate" field as an array, but the API returns an
// object, so the body is decoded here and both shapes are accepted. A 404
// returns core.ErrNotFound.
func getCertificate(
	ctx context.Context,
	m *Meta,
	id string,
) (*core.Certificate, error) {
	client, err := m.rawCore()
	if err != nil {
		return nil, err
	}

	rsp, err := client.GetCertificate(ctx, &core.GetCertificateParams{
		CertificateId: &id,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rsp.Body.Close() }()

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	switch {
	case rsp.StatusCode == http.StatusNotFound:
		return nil, core.ErrNotFound
	case rsp.StatusCode < 200 || rsp.StatusCode >= 300:
		return nil, genericAPIError(core.ErrRequestFailed, body)
	}

	var envelope struct {
		Certificate json.RawMessage `json:"certificate"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode certificate response: %w", err)
	}

	if strings.HasPrefix(strings.TrimSpace(string(envelope.Certificate)), "[") {
		var certs []core.Certificate
		if err := json.Unmarshal(envelope.Certificate, &certs); err != nil {
			return nil, fmt.Errorf("decode certificate response: %w", err)
		}
		if len(certs) == 0 {
			return nil, core.ErrNotFound
		}

		return &certs[0], nil
	}

	var cert core.Certificate
	if err := json.Unmarshal(envelope.Certificate, &cert); err != nil {
		return nil, fmt.Errorf("decode certificate response: %w", err)
	}

	return &cert, nil
}

// createCertificate posts a new certificate for the provider organization and
// returns the API response, which carries the certificate and a nullable
// issuance task.
func createCertificate(
	ctx context.Context,
	m *Meta,
	args core.CertificateArguments,
) (*core.Certificate, nullable.Nullable[core.Task], error) {
	res, err := m.Core.PostOrganizationCertificatesWithResponse(ctx,
		core.PostOrganizationCertificatesJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &m.confOrganization,
			},
			Properties: args,
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		return nil, nil, err
	}
	if res.JSON201 == nil || res.JSON201.Certificate.Id == nil {
		return nil, nil, errors.New(
			"unexpected empty response creating certificate",
		)
	}

	return &res.JSON201.Certificate, res.JSON201.Task, nil
}

// waitForCertificateIssuance waits for the issuance task, when one was
// returned, then reads the certificate and requires it to be issued. When the
// task fails or the certificate is not issued, the error includes the API's
// issue_error, and the certificate read back is returned so callers can
// persist its state.
func waitForCertificateIssuance(
	ctx context.Context,
	m *Meta,
	id string,
	task nullable.Nullable[core.Task],
	timeout time.Duration,
) (*core.Certificate, error) {
	var taskErr error
	if task.IsSpecified() && !task.IsNull() {
		t := task.MustGet()
		if t.Id != nil {
			taskErr = waitForTaskCompletion(ctx, m, timeout, *t.Id)
		}
	}

	cert, err := getCertificate(ctx, m, id)
	if err != nil {
		if taskErr != nil {
			return nil, fmt.Errorf("%w; reading certificate: %w", taskErr, err)
		}

		return nil, err
	}

	if cert.State != nil && *cert.State == core.CertificateStateEnumIssued {
		return cert, nil
	}

	return cert, certificateNotIssuedError(cert, taskErr)
}

func certificateNotIssuedError(cert *core.Certificate, taskErr error) error {
	state := "unknown"
	if cert.State != nil {
		state = string(*cert.State)
	}

	msg := fmt.Sprintf("certificate %s was not issued (state: %s)",
		stringOrEmpty(cert.Id), state)
	if taskErr != nil {
		msg += ": " + taskErr.Error()
	}
	if cert.IssueError.IsSpecified() && !cert.IssueError.IsNull() {
		msg += ": issue_error: " + cert.IssueError.MustGet()
	}

	return errors.New(msg)
}

// deleteCertificate deletes a certificate. A missing certificate is treated as
// already deleted. A 409 means a load balancer rule still references the
// certificate, which is surfaced with rotation guidance.
func deleteCertificate(ctx context.Context, m *Meta, id string) error {
	res, err := m.Core.DeleteCertificateWithResponse(ctx,
		core.DeleteCertificateJSONRequestBody{
			Certificate: core.CertificateLookup{Id: &id},
		})
	if err == nil {
		return nil
	}
	if errors.Is(err, core.ErrNotFound) {
		return nil
	}
	if res == nil {
		return err
	}

	if res.StatusCode() == http.StatusConflict || res.JSON409 != nil {
		return fmt.Errorf(
			"certificate %s cannot be deleted while a load balancer rule "+
				"references it (%w). Remove it from the rule's "+
				"certificate_ids first, or set lifecycle "+
				"create_before_destroy = true on the certificate so "+
				"replacements attach before the old certificate is deleted",
			id, genericAPIError(err, res.Body),
		)
	}

	return genericAPIError(err, res.Body)
}

// checkCertificateIssuer returns an error naming the correct resource type
// when a certificate's issuer does not match the resource importing it.
func checkCertificateIssuer(
	cert *core.Certificate,
	want core.IssuerEnum,
) error {
	if cert.Issuer == nil {
		return fmt.Errorf(
			"certificate %s has no issuer in the API response",
			stringOrEmpty(cert.Id),
		)
	}
	if *cert.Issuer == want {
		return nil
	}

	msg := fmt.Sprintf(
		"certificate %s has issuer %q, but %s manages %q certificates",
		stringOrEmpty(cert.Id), string(*cert.Issuer),
		certificateResourceTypeNames[want], string(want),
	)
	if typeName, ok := certificateResourceTypeNames[*cert.Issuer]; ok {
		msg += fmt.Sprintf("; import it with the %s resource instead", typeName)
	}

	return errors.New(msg)
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
