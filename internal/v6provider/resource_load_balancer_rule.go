package v6provider

import (
	"context"
	"errors"
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
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
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
				string(core.RoundRobinRuleAlgorithm),
			),
			Validators: []validator.String{
				stringvalidator.OneOf(
					string(core.LeastConnectionsRuleAlgorithm),
					string(core.RoundRobinRuleAlgorithm),
					string(core.StickyRuleAlgorithm),
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
					string(core.HTTPProtocol),
					string(core.HTTPSProtocol),
					string(core.TCPProtocol),
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
			Required: true,
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
			Default:  stringdefault.StaticString("HTTP"),
			Validators: []validator.String{
				stringvalidator.AlsoRequires(
					path.MatchRoot("check_enabled"),
				),
				stringvalidator.OneOf(
					string(core.HTTPProtocol),
					string(core.HTTPSProtocol),
					string(core.TCPProtocol),
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

	lbref := core.LoadBalancerRef{
		ID: plan.LoadBalancerID.ValueString(),
	}

	args, diags := buildLoadBalancerRuleCreateArgs(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	lbr, _, err := r.M.Core.LoadBalancerRules.Create(ctx, lbref, args)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Create Error",
			"Error creating LoadBalancerRule: "+err.Error(),
		)
		return
	}

	if err := r.LoadBalancerRuleRead(ctx,
		lbr.ID,
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

	lbrRef := core.LoadBalancerRuleRef{ID: id}
	args, diags := buildLoadBalancerRuleUpdateArgs(ctx, &plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := r.M.Core.LoadBalancerRules.Update(ctx, lbrRef, args)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Update Error",
			"Error updating LoadBalancerRule: "+err.Error(),
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

	_, _, err := r.M.Core.LoadBalancerRules.Delete(
		ctx,
		core.LoadBalancerRuleRef{ID: state.ID.ValueString()},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"LoadBalancerRule Delete Error",
			"Error deleting LoadBalancerRule: "+err.Error(),
		)
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
	lbr, _, err := r.M.Core.LoadBalancerRules.GetByID(ctx,
		id)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

	model.ID = types.StringValue(lbr.ID)
	model.LoadBalancerID = types.StringValue(lbr.LoadBalancer.ID)
	model.Algorithm = types.StringValue(string(lbr.Algorithm))
	model.DestinationPort = types.Int64Value(int64(lbr.DestinationPort))
	model.ListenPort = types.Int64Value(int64(lbr.ListenPort))
	model.Protocol = types.StringValue(string(lbr.Protocol))
	model.ProxyProtocol = types.BoolValue(lbr.ProxyProtocol)
	model.CertificateIDs = types.SetValueMust(
		types.StringType,
		ConvertCoreCertsToTFValues(lbr.Certificates),
	)
	model.BackendSSL = types.BoolValue(lbr.BackendSSL)
	model.PassthroughSSL = types.BoolValue(lbr.PassthroughSSL)
	model.CheckEnabled = types.BoolValue(lbr.CheckEnabled)
	model.CheckFall = types.Int64Value(int64(lbr.CheckFall))
	model.CheckInterval = types.Int64Value(int64(lbr.CheckInterval))
	model.CheckPath = types.StringValue(lbr.CheckPath)
	model.CheckProtocol = types.StringValue(string(lbr.CheckProtocol))
	model.CheckRise = types.Int64Value(int64(lbr.CheckRise))
	model.CheckTimeout = types.Int64Value(int64(lbr.CheckTimeout))
	model.CheckHTTPStatuses = types.StringValue(string(lbr.CheckHTTPStatuses))

	return nil
}

// helpers

func convertCertificateModelsToCertificateRefs(
	ctx context.Context,
	set basetypes.SetValue,
) (*[]core.CertificateRef, diag.Diagnostics) {
	diags := diag.Diagnostics{}

	certList := []types.String{}

	diags.Append(set.ElementsAs(ctx, &certList, true)...)
	if diags.HasError() {
		return nil, diags
	}

	certs := make([]core.CertificateRef, len(certList))
	for i, cert := range certList {
		certs[i] = core.CertificateRef{
			ID: cert.ValueString(),
		}
	}

	return &certs, diags
}

func buildLoadBalancerRuleCreateArgs(
	ctx context.Context,
	r *LoadBalancerRuleResourceModel,
) (*core.LoadBalancerRuleArguments, diag.Diagnostics) {
	args := &core.LoadBalancerRuleArguments{
		Algorithm:         convertAlgorithm(r.Algorithm.ValueString()),
		DestinationPort:   int(r.DestinationPort.ValueInt64()),
		ListenPort:        int(r.ListenPort.ValueInt64()),
		Protocol:          convertProtocol(r.Protocol.ValueString()),
		ProxyProtocol:     r.ProxyProtocol.ValueBoolPointer(),
		CheckEnabled:      r.CheckEnabled.ValueBoolPointer(),
		CheckFall:         int(r.CheckFall.ValueInt64()),
		CheckInterval:     int(r.CheckInterval.ValueInt64()),
		CheckPath:         r.CheckPath.ValueString(),
		CheckProtocol:     convertProtocol(r.CheckProtocol.ValueString()),
		CheckRise:         int(r.CheckRise.ValueInt64()),
		CheckTimeout:      int(r.CheckTimeout.ValueInt64()),
		CheckHTTPStatuses: core.HTTPStatuses(r.CheckHTTPStatuses.ValueString()),
		BackendSSL:        r.BackendSSL.ValueBoolPointer(),
		PassthroughSSL:    r.PassthroughSSL.ValueBoolPointer(),
	}

	certs, diags := convertCertificateModelsToCertificateRefs(
		ctx,
		r.CertificateIDs,
	)
	if diags.HasError() {
		return nil, diags
	}

	args.Certificates = certs

	return args, diags
}

func buildLoadBalancerRuleUpdateArgs(
	ctx context.Context,
	plan *LoadBalancerRuleResourceModel,
	state *LoadBalancerRuleResourceModel,
) (*core.LoadBalancerRuleArguments, diag.Diagnostics) {
	args := &core.LoadBalancerRuleArguments{}

	if !plan.Algorithm.Equal(state.Algorithm) {
		args.Algorithm = convertAlgorithm(plan.Algorithm.ValueString())
	}

	if !plan.DestinationPort.Equal(state.DestinationPort) {
		args.DestinationPort = int(plan.DestinationPort.ValueInt64())
	}

	if !plan.ListenPort.Equal(state.ListenPort) {
		args.ListenPort = int(plan.ListenPort.ValueInt64())
	}

	if !plan.Protocol.Equal(state.Protocol) {
		args.Protocol = convertProtocol(plan.Protocol.ValueString())
	}

	if !plan.ProxyProtocol.Equal(state.ProxyProtocol) {
		args.ProxyProtocol = plan.ProxyProtocol.ValueBoolPointer()
	}

	if !plan.CheckEnabled.Equal(state.CheckEnabled) {
		args.CheckEnabled = plan.CheckEnabled.ValueBoolPointer()
	}

	if !plan.CheckFall.Equal(state.CheckFall) {
		args.CheckFall = int(plan.CheckFall.ValueInt64())
	}

	if !plan.CheckInterval.Equal(state.CheckInterval) {
		args.CheckInterval = int(plan.CheckInterval.ValueInt64())
	}

	if !plan.CheckPath.Equal(state.CheckPath) {
		args.CheckPath = plan.CheckPath.ValueString()
	}

	if !plan.CheckProtocol.Equal(state.CheckProtocol) {
		args.CheckProtocol = convertProtocol(plan.CheckProtocol.ValueString())
	}

	if !plan.CheckRise.Equal(state.CheckRise) {
		args.CheckRise = int(plan.CheckRise.ValueInt64())
	}

	if !plan.CheckTimeout.Equal(state.CheckTimeout) {
		args.CheckTimeout = int(plan.CheckTimeout.ValueInt64())
	}

	if !plan.CheckHTTPStatuses.Equal(state.CheckHTTPStatuses) {
		args.CheckHTTPStatuses = core.HTTPStatuses(
			plan.CheckHTTPStatuses.ValueString(),
		)
	}

	if !plan.BackendSSL.Equal(state.BackendSSL) {
		args.BackendSSL = plan.BackendSSL.ValueBoolPointer()
	}

	if !plan.PassthroughSSL.Equal(state.PassthroughSSL) {
		args.PassthroughSSL = plan.PassthroughSSL.ValueBoolPointer()
	}

	if !plan.CertificateIDs.Equal(state.CertificateIDs) {
		certs, diags := convertCertificateModelsToCertificateRefs(
			ctx,
			plan.CertificateIDs,
		)
		if diags.HasError() {
			return nil, diags
		}

		args.Certificates = certs
	}

	return args, nil
}

func convertAlgorithm(algo string) core.LoadBalancerRuleAlgorithm {
	switch algo {
	case string(core.LeastConnectionsRuleAlgorithm):
		return core.LeastConnectionsRuleAlgorithm
	case string(core.RoundRobinRuleAlgorithm):
		return core.RoundRobinRuleAlgorithm
	case string(core.StickyRuleAlgorithm):
		return core.StickyRuleAlgorithm
	default:
		return core.RoundRobinRuleAlgorithm
	}
}

func convertProtocol(proto string) core.Protocol {
	switch strings.ToUpper(proto) {
	case string(core.HTTPProtocol):
		return core.HTTPProtocol
	case string(core.HTTPSProtocol):
		return core.HTTPSProtocol
	case string(core.TCPProtocol):
		return core.TCPProtocol
	default:
		return core.HTTPProtocol
	}
}
