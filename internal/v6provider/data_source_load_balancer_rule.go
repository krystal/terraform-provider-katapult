package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancerRuleDataSource struct {
		M *Meta
	}

	LoadBalancerRuleDataSourceModel struct {
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

func (ds *LoadBalancerRuleDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rule"
}

func (ds *LoadBalancerRuleDataSource) Configure(
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

	ds.M = meta
}

func loadBalancerRuleDataSourceSchemaAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required: true,
		},
		"load_balancer_id": schema.StringAttribute{
			Computed: true,
		},
		"algorithm": schema.StringAttribute{
			Computed: true,
		},
		"destination_port": schema.Int64Attribute{
			Computed: true,
		},
		"listen_port": schema.Int64Attribute{
			Computed: true,
		},
		"protocol": schema.StringAttribute{
			Computed: true,
		},
		"proxy_protocol": schema.BoolAttribute{
			Computed: true,
		},
		"certificate_ids": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
		},
		"backend_ssl": schema.BoolAttribute{
			Computed: true,
		},
		"passthrough_ssl": schema.BoolAttribute{
			Computed: true,
		},
		"check_enabled": schema.BoolAttribute{
			Computed: true,
		},
		"check_fall": schema.Int64Attribute{
			Computed: true,
		},
		"check_interval": schema.Int64Attribute{
			Computed: true,
		},
		"check_http_statuses": schema.StringAttribute{
			Computed: true,
		},
		"check_path": schema.StringAttribute{
			Computed: true,
		},
		"check_protocol": schema.StringAttribute{
			Computed: true,
		},
		"check_rise": schema.Int64Attribute{
			Computed: true,
		},
		"check_timeout": schema.Int64Attribute{
			Computed: true,
		},
	}
}

func (ds *LoadBalancerRuleDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: loadBalancerRuleDataSourceSchemaAttrs(),
	}
}

func (ds LoadBalancerRuleDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data LoadBalancerRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	res, err := ds.M.Core.GetLoadBalancersRulesLoadBalancerRuleWithResponse(ctx,
		&core.GetLoadBalancersRulesLoadBalancerRuleParams{
			LoadBalancerRuleId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		resp.Diagnostics.AddError(
			"Load Balancer Rule GetByID Error",
			err.Error(),
		)

		return
	}

	if res.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Load Balancer Rule GetByID Error",
			"response body is nil",
		)
	}

	lbr := res.JSON200.LoadBalancerRule

	data.ID = types.StringPointerValue(lbr.Id)
	data.LoadBalancerID = types.StringPointerValue(lbr.LoadBalancer.Id)
	data.Algorithm = types.StringValue(string(*lbr.Algorithm))
	data.DestinationPort = types.Int64Value(int64(*lbr.DestinationPort))
	data.ListenPort = types.Int64Value(int64(*lbr.ListenPort))
	data.Protocol = types.StringValue(string(*lbr.Protocol))
	data.ProxyProtocol = types.BoolPointerValue(lbr.ProxyProtocol)
	data.CertificateIDs = types.SetValueMust(
		types.StringType,
		ConvertCoreCertsToTFValues(*lbr.Certificates),
	)
	data.BackendSSL = types.BoolPointerValue(lbr.BackendSsl)
	data.PassthroughSSL = types.BoolPointerValue(lbr.PassthroughSsl)
	data.CheckEnabled = types.BoolPointerValue(lbr.CheckEnabled)
	data.CheckFall = types.Int64Value(int64(*lbr.CheckFall))
	data.CheckInterval = types.Int64Value(int64(*lbr.CheckInterval))
	data.CheckPath = types.StringPointerValue(lbr.CheckPath)
	data.CheckProtocol = types.StringValue(string(*lbr.CheckProtocol))
	data.CheckRise = types.Int64Value(int64(*lbr.CheckRise))
	data.CheckTimeout = types.Int64Value(int64(*lbr.CheckTimeout))
	data.CheckHTTPStatuses = types.StringValue(string(*lbr.CheckHttpStatuses))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
