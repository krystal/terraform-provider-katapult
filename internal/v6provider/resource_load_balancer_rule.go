package v6provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancerRuleResource struct {
		M *Meta
	}

	LoadBalancerRuleResourceModel struct {
		ID                types.String `tfsdk:"id"`
		LoadBalancerID    types.String `tfsdk:"load_balancer_id"`
		Algorithm         types.String `tfsdk:"algorithm"`
		DestinationPort   types.Int64  `tfsdk:"destination_port"`
		ListenPort        types.Int64  `tfsdk:"listen_port"`
		Protocol          types.String `tfsdk:"protocol"`
		ProxyProtocol     types.Bool   `tfsdk:"proxy_protocol"`
		CertificateIDs    types.Set    `tfsdk:"certificate_ids"`
		BackendSSL        types.Bool   `tfsdk:"backend_ssl"`
		PassthroughSSL    types.Bool   `tfsdk:"passthrough_ssl"`
		CheckEnabled      types.Bool   `tfsdk:"check_enabled"`
		CheckFall         types.Int64  `tfsdk:"check_fall"`
		CheckInterval     types.Int64  `tfsdk:"check_interval"`
		CheckHTTPStatuses types.String `tfsdk:"check_http_statuses"`
		CheckPath         types.String `tfsdk:"check_path"`
		CheckProtocol     types.String `tfsdk:"check_protocol"`
		CheckRise         types.Int64  `tfsdk:"check_rise"`
		CheckTimeout      types.Int64  `tfsdk:"check_timeout"`
	}
)

func ptr[T any](v T) *T {
	return &v
}

func (r *LoadBalancerRuleResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rule"
}

func (r *LoadBalancerRuleResource) Configure(
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

func LoadBalancerRuleType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":                  types.StringType,
			"load_balancer_id":    types.StringType,
			"algorithm":           types.StringType,
			"destination_port":    types.Int64Type,
			"listen_port":         types.Int64Type,
			"protocol":            types.StringType,
			"proxy_protocol":      types.BoolType,
			"certificate_ids":     types.SetType{ElemType: types.StringType},
			"backend_ssl":         types.BoolType,
			"passthrough_ssl":     types.BoolType,
			"check_enabled":       types.BoolType,
			"check_fall":          types.Int64Type,
			"check_interval":      types.Int64Type,
			"check_http_statuses": types.StringType,
			"check_path":          types.StringType,
			"check_protocol":      types.StringType,
			"check_rise":          types.Int64Type,
			"check_timeout":       types.Int64Type,
		},
	}
}

//nolint:funlen
func LoadBalancerRuleSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true,
		},
		"load_balancer_id": schema.StringAttribute{
			Required: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"algorithm": schema.StringAttribute{
			Computed: true,
			Optional: true,
			Default: stringdefault.StaticString(
				string(core.RoundRobin),
			),
			Validators: []validator.String{
				stringvalidator.OneOf(
					string(core.LeastConnections),
					string(core.RoundRobin),
					string(core.Sticky),
				),
			},
		},
		"destination_port": schema.Int64Attribute{
			Required: true,
		},
		"listen_port": schema.Int64Attribute{
			Required: true,
		},
		"protocol": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf(
					string(core.LoadBalancerRuleProtocolEnumHTTP),
					string(core.LoadBalancerRuleProtocolEnumHTTPS),
					string(core.LoadBalancerRuleProtocolEnumTCP),
				),
			},
		},
		"proxy_protocol": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
		},
		"certificate_ids": schema.SetAttribute{
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
		},
		"backend_ssl": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
		},
		"passthrough_ssl": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
		},
		"check_enabled": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
		},
		"check_fall": schema.Int64Attribute{
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(2),
			Validators: []validator.Int64{
				int64validator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
				int64validator.AtLeast(1),
			},
		},
		"check_interval": schema.Int64Attribute{
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(20),
			Validators: []validator.Int64{
				int64validator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
				int64validator.AtLeast(1),
			},
		},
		"check_http_statuses": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Default:  stringdefault.StaticString("2"),
			Validators: []validator.String{
				stringvalidator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
			},
		},
		"check_path": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Default:  stringdefault.StaticString("/"),
			Validators: []validator.String{
				stringvalidator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
				stringvalidator.LengthAtLeast(1),
			},
		},
		"check_protocol": schema.StringAttribute{
			Optional: true,
			Computed: true,
			Default: stringdefault.StaticString(
				string(core.LoadBalancerRuleCheckProtocolEnumHTTP),
			),
			Validators: []validator.String{
				stringvalidator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
				stringvalidator.OneOf(
					string(core.LoadBalancerRuleCheckProtocolEnumHTTP),
					string(core.LoadBalancerRuleCheckProtocolEnumTCP),
				),
			},
		},
		"check_rise": schema.Int64Attribute{
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(2),
			Validators: []validator.Int64{
				int64validator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
			},
		},
		"check_timeout": schema.Int64Attribute{
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(5),
			Validators: []validator.Int64{
				int64validator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
			},
		},
	}
}

func (r LoadBalancerRuleResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: LoadBalancerRuleSchemaAttributes(),
	}
}

func (r LoadBalancerRuleResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var data LoadBalancerRuleResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	proto := data.Protocol.ValueString()
	checkProto := data.CheckProtocol.ValueString()

	if data.CheckPath.ValueStringPointer() != nil {
		if checkProto != "HTTP" {
			resp.Diagnostics.AddError(
				"check_path",
				"check_path cannot be set if check_protocol is not HTTP",
			)
		}
	}

	if data.CheckHTTPStatuses.ValueStringPointer() != nil {
		if checkProto != "HTTP" {
			resp.Diagnostics.AddError(
				"check_http_statuses",
				"check_http_statuses cannot be set if "+
					"check_protocol is not HTTP",
			)
		}
	}

	if !data.CertificateIDs.IsNull() && proto != "HTTPS" {
		resp.Diagnostics.AddError(
			"certificate_ids",
			"certificate_ids cannot be set if protocol is not HTTPS",
		)
	}

	if data.PassthroughSSL.ValueBool() && proto != "HTTPS" {
		resp.Diagnostics.AddError(
			"passthrough_ssl",
			"passthrough_ssl cannot be set if protocol is not HTTPS",
		)
	}
}

func (r *LoadBalancerRuleResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan LoadBalancerRuleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	args, diags := buildLoadBalancerRuleCreateArgs(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	lbrRes, err := r.M.Core.PostLoadBalancerRulesWithResponse(ctx,
		core.PostLoadBalancerRulesJSONRequestBody{
			LoadBalancer: core.LoadBalancerLookup{
				Id: plan.LoadBalancerID.ValueStringPointer(),
			},
			Properties: args,
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Create Error",
			"Error creating LoadBalancerRule: "+err.Error(),
		)
		return
	}

	if lbrRes.JSON200 == nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Create Error",
			"response body is nil",
		)
		return
	}

	lbr := lbrRes.JSON200.LoadBalancerRule

	if err := r.LoadBalancerRuleRead(ctx,
		*lbr.Id,
		&plan,
		&resp.State,
	); err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Read Error",
			"Error reading LoadBalancerRule: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LoadBalancerRuleResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	state := &LoadBalancerRuleResourceModel{}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.LoadBalancerRuleRead(ctx,
		state.ID.ValueString(),
		state,
		&resp.State,
	); err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Read Error",
			"Error reading LoadBalancerRule: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *LoadBalancerRuleResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan LoadBalancerRuleResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state LoadBalancerRuleResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	args, diags := buildLoadBalancerRuleUpdateArgs(ctx, &plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.
		PatchLoadBalancersRulesLoadBalancerRuleWithResponse(ctx,
			core.PatchLoadBalancersRulesLoadBalancerRuleJSONRequestBody{
				LoadBalancerRule: core.LoadBalancerRuleLookup{
					Id: &id,
				},
				Properties: args,
			},
		)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Update Error",
			"Error updating LoadBalancerRule: "+err.Error(),
		)
		return
	}

	if res.JSON200 == nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Update Error",
			"response body is nil",
		)
		return
	}

	if plan.LoadBalancerID.IsNull() {
		plan.LoadBalancerID = state.LoadBalancerID
	}

	if err := r.LoadBalancerRuleRead(
		ctx,
		id,
		&plan,
		&resp.State,
	); err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Read Error",
			"Error reading LoadBalancerRule: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LoadBalancerRuleResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	state := &LoadBalancerRuleResourceModel{}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.
		DeleteLoadBalancersRulesLoadBalancerRuleWithResponse(ctx,
			core.DeleteLoadBalancersRulesLoadBalancerRuleJSONRequestBody{
				LoadBalancerRule: core.LoadBalancerRuleLookup{
					Id: state.ID.ValueStringPointer(),
				},
			},
		)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Delete Error",
			"Error deleting LoadBalancerRule: "+err.Error(),
		)
	}

	if res.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Delete Error",
			"response status code is not 200",
		)
		return
	}
}

func (r *LoadBalancerRuleResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *LoadBalancerRuleResource) LoadBalancerRuleRead(
	ctx context.Context,
	id string,
	model *LoadBalancerRuleResourceModel,
	state *tfsdk.State,
) error {
	res, err := r.M.Core.GetLoadBalancersRulesLoadBalancerRuleWithResponse(ctx,
		&core.GetLoadBalancersRulesLoadBalancerRuleParams{
			LoadBalancerRuleId: &id,
		},
	)
	if res.StatusCode() == http.StatusNotFound {
		state.RemoveResource(ctx)

		return nil
	}

	if err != nil {
		return err
	}

	if res.JSON200 == nil {
		return errors.New("response body is nil")
	}

	lbr := res.JSON200.LoadBalancerRule

	model.ID = types.StringPointerValue(lbr.Id)
	model.LoadBalancerID = types.StringPointerValue(lbr.LoadBalancer.Id)
	model.Algorithm = types.StringValue(string(*lbr.Algorithm))
	model.DestinationPort = types.Int64Value(int64(*lbr.DestinationPort))
	model.ListenPort = types.Int64Value(int64(*lbr.ListenPort))
	model.Protocol = types.StringValue(string(*lbr.Protocol))
	model.ProxyProtocol = types.BoolValue(*lbr.ProxyProtocol)
	model.CertificateIDs = types.SetValueMust(
		types.StringType,
		ConvertCoreCertsToTFValues(*lbr.Certificates),
	)
	model.BackendSSL = types.BoolPointerValue(lbr.BackendSsl)
	model.PassthroughSSL = types.BoolPointerValue(lbr.PassthroughSsl)
	model.CheckEnabled = types.BoolPointerValue(lbr.CheckEnabled)
	model.CheckFall = types.Int64Value(int64(*lbr.CheckFall))
	model.CheckInterval = types.Int64Value(int64(*lbr.CheckInterval))
	model.CheckHTTPStatuses = types.StringValue(string(*lbr.CheckHttpStatuses))
	model.CheckPath = types.StringPointerValue(lbr.CheckPath)
	model.CheckProtocol = types.StringValue(string(*lbr.CheckProtocol))
	model.CheckRise = types.Int64Value(int64(*lbr.CheckRise))
	model.CheckTimeout = types.Int64Value(int64(*lbr.CheckTimeout))

	return nil
}

// helpers

func convertCertificateModelsToCertificateLookups(
	ctx context.Context,
	set basetypes.SetValue,
) (*[]core.CertificateLookup, diag.Diagnostics) {
	diags := diag.Diagnostics{}

	certList := []types.String{}

	diags.Append(set.ElementsAs(ctx, &certList, true)...)
	if diags.HasError() {
		return nil, diags
	}

	certs := make([]core.CertificateLookup, len(certList))
	for i, cert := range certList {
		certs[i] = core.CertificateLookup{
			Id: cert.ValueStringPointer(),
		}
	}

	return &certs, diags
}

func buildLoadBalancerRuleCreateArgs(
	ctx context.Context,
	r *LoadBalancerRuleResourceModel,
) (core.LoadBalancerRuleArguments, diag.Diagnostics) {
	args := core.LoadBalancerRuleArguments{
		Algorithm:       ptr(convertAlgorithm(r.Algorithm.ValueString())),
		DestinationPort: ptr(int(r.DestinationPort.ValueInt64())),
		ListenPort:      ptr(int(r.ListenPort.ValueInt64())),
		Protocol:        ptr(convertProtocol(r.Protocol.ValueString())),
		ProxyProtocol:   r.ProxyProtocol.ValueBoolPointer(),
		BackendSsl:      r.BackendSSL.ValueBoolPointer(),
		PassthroughSsl:  r.PassthroughSSL.ValueBoolPointer(),
		CheckEnabled:    r.CheckEnabled.ValueBoolPointer(),
		CheckFall:       ptr(int(r.CheckFall.ValueInt64())),
		CheckInterval:   ptr(int(r.CheckInterval.ValueInt64())),
		CheckPath:       r.CheckPath.ValueStringPointer(),
		CheckProtocol: ptr(
			convertCheckProtocol(r.CheckProtocol.ValueString()),
		),
		CheckRise:    ptr(int(r.CheckRise.ValueInt64())),
		CheckTimeout: ptr(int(r.CheckTimeout.ValueInt64())),
		CheckHttpStatuses: ptr(core.LoadBalancerRuleHTTPStatusesEnum(
			r.CheckHTTPStatuses.ValueString(),
		)),
	}

	certs, diags := convertCertificateModelsToCertificateLookups(
		ctx,
		r.CertificateIDs,
	)
	if diags.HasError() {
		return args, diags
	}

	args.Certificates = certs

	return args, diags
}

func buildLoadBalancerRuleUpdateArgs(
	ctx context.Context,
	plan *LoadBalancerRuleResourceModel,
	state *LoadBalancerRuleResourceModel,
) (core.LoadBalancerRuleArguments, diag.Diagnostics) {
	args := core.LoadBalancerRuleArguments{}

	if !plan.Algorithm.Equal(state.Algorithm) {
		args.Algorithm = ptr(convertAlgorithm(plan.Algorithm.ValueString()))
	}

	if !plan.DestinationPort.Equal(state.DestinationPort) {
		args.DestinationPort = ptr(int(plan.DestinationPort.ValueInt64()))
	}

	if !plan.ListenPort.Equal(state.ListenPort) {
		args.ListenPort = ptr(int(plan.ListenPort.ValueInt64()))
	}

	if !plan.Protocol.Equal(state.Protocol) {
		args.Protocol = ptr(convertProtocol(plan.Protocol.ValueString()))
	}

	if !plan.ProxyProtocol.Equal(state.ProxyProtocol) {
		args.ProxyProtocol = plan.ProxyProtocol.ValueBoolPointer()
	}

	if !plan.BackendSSL.Equal(state.BackendSSL) {
		args.BackendSsl = plan.BackendSSL.ValueBoolPointer()
	}

	if !plan.PassthroughSSL.Equal(state.PassthroughSSL) {
		args.PassthroughSsl = plan.PassthroughSSL.ValueBoolPointer()
	}

	if !plan.CheckEnabled.Equal(state.CheckEnabled) {
		args.CheckEnabled = plan.CheckEnabled.ValueBoolPointer()
	}

	if !plan.CheckFall.Equal(state.CheckFall) {
		args.CheckFall = ptr(int(plan.CheckFall.ValueInt64()))
	}

	if !plan.CheckInterval.Equal(state.CheckInterval) {
		args.CheckInterval = ptr(int(plan.CheckInterval.ValueInt64()))
	}

	if !plan.CheckPath.Equal(state.CheckPath) {
		args.CheckPath = plan.CheckPath.ValueStringPointer()
	}

	if !plan.CheckProtocol.Equal(state.CheckProtocol) {
		args.CheckProtocol = ptr(
			convertCheckProtocol(plan.CheckProtocol.ValueString()),
		)
	}

	if !plan.CheckRise.Equal(state.CheckRise) {
		args.CheckRise = ptr(int(plan.CheckRise.ValueInt64()))
	}

	if !plan.CheckTimeout.Equal(state.CheckTimeout) {
		args.CheckTimeout = ptr(int(plan.CheckTimeout.ValueInt64()))
	}

	if !plan.CheckHTTPStatuses.Equal(state.CheckHTTPStatuses) {
		args.CheckHttpStatuses = ptr(core.LoadBalancerRuleHTTPStatusesEnum(
			plan.CheckHTTPStatuses.ValueString(),
		))
	}

	if !plan.CertificateIDs.Equal(state.CertificateIDs) {
		certs, diags := convertCertificateModelsToCertificateLookups(
			ctx,
			plan.CertificateIDs,
		)
		if diags.HasError() {
			return args, diags
		}

		args.Certificates = certs
	}

	return args, nil
}

func convertAlgorithm(algo string) core.LoadBalancerRuleAlgorithmEnum {
	switch algo {
	case string(core.LeastConnections):
		return core.LeastConnections
	case string(core.RoundRobin):
		return core.RoundRobin
	case string(core.Sticky):
		return core.Sticky
	default:
		return core.RoundRobin
	}
}

func convertProtocol(proto string) core.LoadBalancerRuleProtocolEnum {
	switch strings.ToUpper(proto) {
	case string(core.LoadBalancerRuleProtocolEnumHTTP):
		return core.LoadBalancerRuleProtocolEnumHTTP
	case string(core.LoadBalancerRuleProtocolEnumHTTPS):
		return core.LoadBalancerRuleProtocolEnumHTTPS
	case string(core.LoadBalancerRuleProtocolEnumTCP):
		return core.LoadBalancerRuleProtocolEnumTCP
	default:
		return core.LoadBalancerRuleProtocolEnumHTTP
	}
}

func convertCheckProtocol(proto string) core.LoadBalancerRuleCheckProtocolEnum {
	switch strings.ToUpper(proto) {
	case string(core.LoadBalancerRuleCheckProtocolEnumHTTP):
		return core.LoadBalancerRuleCheckProtocolEnumHTTP
	case string(core.LoadBalancerRuleCheckProtocolEnumTCP):
		return core.LoadBalancerRuleCheckProtocolEnumTCP
	default:
		return core.LoadBalancerRuleCheckProtocolEnumHTTP
	}
}
